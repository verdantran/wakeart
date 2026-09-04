package main

import (
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"

	"github.com/verdantran/wakeart"
	"github.com/verdantran/wakeart/internal/awake"
	"github.com/verdantran/wakeart/internal/config"
	"github.com/verdantran/wakeart/internal/effect"
	"github.com/verdantran/wakeart/internal/palette"
	"github.com/verdantran/wakeart/internal/render"
	"github.com/verdantran/wakeart/internal/scene"
	"github.com/verdantran/wakeart/internal/tui"
)

const usage = `wakeart — ASCII art on a loop, for a spare terminal pane.

wakeart [flags]              Start the carousel

Flags:
  --scene <name>             Start on a specific scene
  --only <tag>               Restrict the deck to a tag (repeatable)
  --shuffle                  Random order
  --duration <dur>           Seconds per scene, overriding each scene's own hold
  --speed <float>            Global animation multiplier (default 1.0)
  --palette <name>           Override palette for all scenes
  --effects <list|none>      Override effect set
  --intensity <level>        off | subtle | heavy
  --scene-dir <path>         Additional scene directory (repeatable)
  --once                     Print one frame to stdout and exit
  --seed <int>               Deterministic run
  --no-carousel              Start held on one scene instead of auto-advancing
  --awake, --caffeinate      Stop the system sleeping while wakeart runs
  --no-color                 Monochrome
  --on-blur <mode>           When the terminal loses focus: run | throttle | pause
  --config <path>            Alternate config file

Commands:
  wakeart list               Table of scenes: name, source, frames, size, tags
  wakeart new <name>         Scaffold a scene file in the user dir and open $EDITOR
  wakeart add <file>         Copy a file into the user scene dir, inferring metadata
  wakeart show <name>        Render one scene once, non-interactively
  wakeart doctor             Terminal capabilities + scene validation report
`

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "wakeart:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// Subcommands come first so their own arguments are not eaten by the
	// carousel's flag set.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, rest := args[0], args[1:]
		switch cmd {
		case "list":
			return cmdList(rest)
		case "new":
			return cmdNew(rest)
		case "add":
			return cmdAdd(rest)
		case "show":
			return cmdShow(rest)
		case "doctor":
			return cmdDoctor(rest)
		case "help":
			fmt.Print(usage)
			return nil
		default:
			return fmt.Errorf("unknown command %q (try: wakeart help)", cmd)
		}
	}

	fs := flag.NewFlagSet("wakeart", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(usage) }
	var (
		sceneName  = fs.String("scene", "", "")
		only       multiFlag
		sceneDirs  multiFlag
		shuffle    = fs.Bool("shuffle", false, "")
		duration   = fs.String("duration", "", "")
		speed      = fs.Float64("speed", 0, "")
		pal        = fs.String("palette", "", "")
		effects    = fs.String("effects", "", "")
		intensity  = fs.String("intensity", "", "")
		once       = fs.Bool("once", false, "")
		seed       = fs.Int64("seed", 0, "")
		noColor    = fs.Bool("no-color", false, "")
		noCaro     = fs.Bool("no-carousel", false, "")
		awakeOn    = fs.Bool("awake", false, "")
		caffeinate = fs.Bool("caffeinate", false, "")
		onBlur     = fs.String("on-blur", "", "")
		cfgPath    = fs.String("config", "", "")
	)
	fs.Var(&only, "only", "")
	fs.Var(&sceneDirs, "scene-dir", "")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	if *duration != "" {
		d, err := time.ParseDuration(*duration)
		if err != nil {
			return fmt.Errorf("duration: %w", err)
		}
		cfg.HoldDuration = d
		cfg.HoldForced = true
	}
	if *shuffle {
		cfg.Shuffle = true
	}
	if *speed > 0 {
		cfg.Speed = *speed
	}
	if *pal != "" {
		if _, ok := palette.Get(*pal); !ok {
			return fmt.Errorf("unknown palette %q (have: %s)", *pal, strings.Join(palette.Names(), ", "))
		}
		cfg.Palette = *pal
	}
	if *effects != "" {
		names, in, err := parseEffects(*effects)
		if err != nil {
			return err
		}
		cfg.Effects = names
		if in != "" {
			cfg.Intensity = in
		}
	}
	if *intensity != "" {
		cfg.Intensity = *intensity
	}
	if *noCaro {
		cfg.Carousel = false
	}
	if *awakeOn || *caffeinate {
		cfg.Awake = true
	}
	if *onBlur != "" {
		if tui.ParseBlurMode(*onBlur).String() != strings.ToLower(*onBlur) {
			return fmt.Errorf("unknown --on-blur %q (have: run, throttle, pause)", *onBlur)
		}
		cfg.OnBlur = *onBlur
	}

	reg := loadRegistry(append(cfg.SceneDirs, sceneDirs...))
	reg.FilterTags(only)
	if reg.Len() == 0 {
		return fmt.Errorf("no scenes found")
	}

	start := 0
	if *sceneName != "" {
		s, i := reg.Find(*sceneName)
		if s == nil {
			return fmt.Errorf("no scene named %q", *sceneName)
		}
		start = i
	}

	mode := palette.Detect()
	if *noColor {
		mode = palette.Mono
	}

	if *once {
		return printOnce(reg, start, cfg, mode)
	}

	sd := *seed
	if sd == 0 {
		sd = time.Now().UnixNano()
	}

	m := tui.New(tui.Options{
		Registry: reg, Config: cfg, Mode: mode, Start: start, Seed: sd,
		NoEffects: *noColor,
	})
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus())
	final, err := p.Run()
	if fm, ok := final.(tui.Model); ok {
		fm.Cleanup()
	}

	// Malformed scenes are reported once the alt screen is gone, so the
	// warning is still on screen afterwards rather than wiped by the carousel.
	for _, e := range reg.Errs {
		fmt.Fprintln(os.Stderr, "wakeart: skipped", e)
	}
	return err
}

func loadRegistry(extra []string) *scene.Registry {
	dirs := []string{config.SceneDir()}
	dirs = append(dirs, extra...)
	return scene.Load(wakeart.Scenes(), dirs)
}

func parseEffects(v string) (names []string, intensity string, err error) {
	if v == "none" || v == "off" {
		return nil, "off", nil
	}
	for _, n := range strings.Split(v, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if !effect.Valid(n) {
			return nil, "", fmt.Errorf("unknown effect %q (have: %s)", n, strings.Join(effect.Names, ", "))
		}
		names = append(names, n)
	}
	return names, "", nil
}

// viewport is the drawing area for the non-interactive paths. Procedural
// scenes need one; art read from a file brings its own size.
func viewport() (int, int) {
	if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 0 && h > 0 {
		return w, h - 1
	}
	return 80, 24
}

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// printOnce is the shell-startup path. It emits no escape sequences when
// stdout is not a terminal.
func printOnce(reg *scene.Registry, start int, cfg config.Config, mode palette.Mode) error {
	idx := start
	if cfg.Shuffle {
		idx = rand.Intn(reg.Len())
	}
	s := reg.Scenes[idx]
	var f *render.Frame
	if s.Procedural() {
		w, h := viewport()
		f = s.Render(1700*time.Millisecond, 1, w, h)
	} else {
		frames, err := s.Frames()
		if err != nil {
			return err
		}
		f = frames[0]
	}
	if !isTTY(os.Stdout) {
		fmt.Println(render.Plain(f))
		return nil
	}
	p, ok := palette.Get(cfg.Palette)
	if !ok {
		p = palette.All[0]
	}
	if s.Meta.Palette != "" && mode != palette.Mono {
		if sp, ok := palette.Get(s.Meta.Palette); ok {
			p = sp
		}
	}
	var b strings.Builder
	render.Paint(f, p, mode, &b)
	fmt.Println(strings.ReplaceAll(b.String(), "\r\n", "\n"))
	return nil
}

func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	var dirs multiFlag
	fs.Var(&dirs, "scene-dir", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	reg := loadRegistry(dirs)
	w := tabber{}
	w.row("NAME", "FRAMES", "SIZE", "COLOUR", "TAGS", "SOURCE")
	for _, s := range reg.Scenes {
		w.row(s.Meta.Name, frameCount(s), sizeOf(s),
			s.ColourPath(), strings.Join(s.Meta.Tags, ","), shortPath(s.Meta.Source))
	}
	w.flush(os.Stdout)
	for _, e := range reg.Errs {
		fmt.Fprintln(os.Stderr, "skipped:", e)
	}
	return nil
}

// frameCount and sizeOf describe a scene for the tables. Procedural scenes
// have neither a frame count nor a fixed size; they are drawn to the viewport.
func frameCount(s *scene.Scene) string {
	if s.Procedural() {
		return "live"
	}
	return fmt.Sprint(len(mustFrames(s)))
}

func sizeOf(s *scene.Scene) string {
	if s.Procedural() {
		return "viewport"
	}
	w, h := s.Size()
	return fmt.Sprintf("%dx%d", w, h)
}

// shortPath keeps the table readable when scenes live under a deep home path.
func shortPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if rest, ok := strings.CutPrefix(p, home); ok {
		return "~" + rest
	}
	return p
}

func mustFrames(s *scene.Scene) []*render.Frame {
	f, err := s.Frames()
	if err != nil {
		return nil
	}
	return f
}

const template = `---
name    = "%s"
palette = "neon"
fps     = 8
loop    = "forward"
tags    = []
---
   your art here
===
   your second frame
`

func cmdNew(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: wakeart new <name>")
	}
	name := args[0]
	dir := config.SceneDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, sanitise(name)+scene.Ext)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	body := fmt.Sprintf(template, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	fmt.Println(path)
	if ed := os.Getenv("EDITOR"); ed != "" {
		c := exec.Command(ed, path)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		return c.Run()
	}
	return nil
}

func cmdAdd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: wakeart add <file>")
	}
	src := args[0]
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	dir := config.SceneDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	base := sanitise(strings.TrimSuffix(filepath.Base(src), filepath.Ext(src)))
	dst := filepath.Join(dir, base+scene.Ext)
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s already exists", dst)
	}

	body := string(data)
	if !strings.HasPrefix(body, "---\n") {
		title := strings.ToUpper(base[:1]) + strings.ReplaceAll(base[1:], "-", " ")
		body = fmt.Sprintf("---\nname = %q\n---\n", title) + body
	}
	if err := os.WriteFile(dst, []byte(body), 0o644); err != nil {
		return err
	}
	s, err := scene.Parse(dst, []byte(body))
	if err != nil {
		return fmt.Errorf("copied to %s but it does not parse: %w", dst, err)
	}
	w, h := s.Size()
	fmt.Printf("%s  %q  %d frames  %dx%d  %s\n", dst, s.Meta.Name, len(mustFrames(s)), w, h, s.ColourPath())
	return nil
}

func sanitise(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "scene"
	}
	return out
}

func cmdShow(args []string) error {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	var dirs multiFlag
	fs.Var(&dirs, "scene-dir", "")
	noColor := fs.Bool("no-color", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: wakeart show <name>")
	}
	reg := loadRegistry(dirs)
	s, _ := reg.Find(fs.Arg(0))
	if s == nil {
		return fmt.Errorf("no scene named %q", fs.Arg(0))
	}
	var f *render.Frame
	if s.Procedural() {
		w, h := viewport()
		f = s.Render(1700*time.Millisecond, 1, w, h)
	} else {
		frames, err := s.Frames()
		if err != nil {
			return err
		}
		f = frames[0]
	}
	mode := palette.Detect()
	if *noColor || !isTTY(os.Stdout) {
		fmt.Println(render.Plain(f))
		return nil
	}
	p, ok := palette.Get(s.Meta.Palette)
	if !ok {
		p = palette.All[0]
	}
	var b strings.Builder
	render.Paint(f, p, mode, &b)
	fmt.Println(strings.ReplaceAll(b.String(), "\r\n", "\n"))
	return nil
}

func cmdDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	var dirs multiFlag
	fs.Var(&dirs, "scene-dir", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mode := palette.Detect()
	fmt.Println("terminal")
	fmt.Printf("  colour mode   %s (%s)\n", mode, palette.DetectReason())
	fmt.Printf("  TERM          %s\n", os.Getenv("TERM"))
	fmt.Printf("  COLORTERM     %s\n", os.Getenv("COLORTERM"))
	fmt.Printf("  stdout is tty %v\n", isTTY(os.Stdout))
	fmt.Printf("  sleep inhibit %s\n", awake.Name())
	fmt.Println()
	fmt.Println("paths")
	fmt.Printf("  config        %s\n", config.Path())
	fmt.Printf("  scenes        %s\n", config.SceneDir())
	fmt.Println()

	reg := loadRegistry(dirs)
	fmt.Printf("scenes (%d)\n", reg.Len())
	w := tabber{indent: "  "}
	w.row("NAME", "FRAMES", "SIZE", "COLOUR", "PALETTE", "SOURCE")
	names := make([]string, 0, reg.Len())
	for _, s := range reg.Scenes {
		names = append(names, s.Meta.Name)
		pal := s.Meta.Palette
		if pal == "" {
			pal = "-"
		} else if _, ok := palette.Get(pal); !ok {
			pal += " (unknown!)"
		}
		w.row(s.Meta.Name, frameCount(s), sizeOf(s),
			s.ColourPath(), pal, shortPath(s.Meta.Source))
	}
	sort.Strings(names)
	w.flush(os.Stdout)

	if len(reg.Errs) > 0 {
		fmt.Println()
		fmt.Printf("errors (%d)\n", len(reg.Errs))
		for _, e := range reg.Errs {
			fmt.Println("  ", e)
		}
	}
	return nil
}

// tabber is a minimal column formatter; the report is the only place we need one.
type tabber struct {
	indent string
	rows   [][]string
}

func (t *tabber) row(cells ...string) { t.rows = append(t.rows, cells) }

func (t *tabber) flush(w io.Writer) {
	if len(t.rows) == 0 {
		return
	}
	widths := make([]int, len(t.rows[0]))
	for _, r := range t.rows {
		for i, c := range r {
			if i < len(widths) && len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	for _, r := range t.rows {
		var b strings.Builder
		b.WriteString(t.indent)
		for i, c := range r {
			if i == len(r)-1 {
				b.WriteString(c)
				break
			}
			b.WriteString(fmt.Sprintf("%-*s  ", widths[i], c))
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}
}
