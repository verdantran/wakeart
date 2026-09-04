# wakeart — spec

> Binary name: `wakeart`. Module path `github.com/<you>/wakeart`, config at `~/.config/wakeart/`.

A terminal toy that plays ASCII art on a loop. You leave it running in a split pane while you work. It looks like the inside of a cyberpunk terminal, it costs you nothing in CPU, and adding new art is dropping a text file in a folder.

---

## 1. Product

### What it is

A single Go binary. Run `wakeart`, get a full-screen carousel of ASCII scenes that advance on a timer, coloured with neon gradients and lightly corrupted with scanlines and glitch bursts. Arrow keys move between scenes, space pauses, `q` quits and leaves your terminal exactly as it was.

### Who it's for

A developer with a wide monitor and a spare tmux pane. The app is ambient — it must never demand attention, never steal focus, never spike a fan, and never leave the terminal in a broken state.

### Non-goals

- Not an image-to-ASCII converter. Art comes in as text, already made.
- Not a video player. No sound, no external media.
- Not a screensaver daemon. No background process, no auto-launch, no lock screen.
- Not a scripting environment. Scenes are data, not programs.
- No network access, ever. Nothing phones home, nothing downloads art at runtime.

### Design constraints

These are the ones that will actually bite, so they're stated up front:

1. **Idle cost is the product.** If it burns 4% CPU it gets closed and never reopened. Target under 1% on an M-series Mac at default settings, and genuinely 0% when the terminal isn't focused.
2. **Restores cleanly.** Alt screen in, alt screen out. A crash must still restore the cursor and reset SGR state — the panic handler runs the teardown.
3. **Adding art requires no build step.** If a friend sends you a chunk of ASCII, it should be running in under thirty seconds.
4. **Degrades down to a dumb terminal.** Truecolor is the good path; 256, 16, and no-colour all have to look deliberate rather than broken.

---

## 2. Behaviour

### Startup

```
$ wakeart
```

Enters the alt screen, hides the cursor, enables focus reporting and bracketed paste-off. Loads embedded scenes plus anything in the user scene directory, shuffles if configured, and starts on the first scene. Startup budget: 50ms to first frame.

### The screen

```
┌──────────────────────────────────────────────────────────┐
│                                                          │
│                                                          │
│                 ▄▄▄  ▄▄▄▄▄  ▄▄                           │
│                █░░█  █░░░█  █░█    (scene, centred)      │
│                ▀▀▀▀  ▀▀▀▀▀  ▀▀                           │
│                                                          │
│                                                          │
│  ▸ NEON SKYLINE                    03/14  ▮▮▮▮▮▯▯▯  ⏵   │
└──────────────────────────────────────────────────────────┘
```

The scene is centred in the viewport. The status bar is one dim line pinned to the bottom: scene name, position in the deck, a progress bar counting down to the next scene, and a transport glyph (`⏵` playing, `⏸` paused, `⤨` shuffle, `⊘` carousel held, `☼` sleep inhibited). It's on by default and hides with `i`. A toggle that would otherwise be invisible borrows the bar for a couple of seconds to say what it did, even when the bar is hidden.

### Keys

| Key | Action |
|---|---|
| `space` | Pause / resume |
| `n`, `→`, `l` | Next scene |
| `p`, `←` | Previous scene |
| `a` | Toggle the carousel — hold on one scene, or auto-advance |
| `k` | Toggle keep-awake — hold off system sleep |
| `s` | Toggle shuffle |
| `c` | Cycle palette |
| `e` | Cycle effect intensity — off / subtle / heavy |
| `E`, `x` | Cycle which effect runs — the configured set, then each effect on its own |
| `+` / `-` | Speed up / slow down (0.25×–4×) |
| `f` | Toggle fullscreen fit (fit-to-width scaling on/off) |
| `i` | Toggle status bar |
| `h`, `?` | Toggle the help overlay |
| `q`, `esc`, `ctrl+c` | Quit |

Every binding is remappable in config. The status bar carries a `h help` hint (the first key bound to help, so it follows a remap) whenever the bar is wide enough for it, and the overlay itself is generated from the keymap, so neither can drift.

### Keeping the machine awake

Art on a spare monitor is worth nothing if the display sleeps on top of it, so `k` toggles the system's own sleep inhibitor: `caffeinate -d -i` on macOS, `systemd-inhibit --what=idle:sleep` on Linux. It is off by default — holding a machine awake is not something to do without being asked — and `--awake` or `awake = true` starts with it on.

The helper runs as a child process tied to wakeart's pid (`caffeinate -w`), in its own process group, so it is cleaned up on quit and dies with wakeart even if wakeart is killed outright. Where no inhibitor exists the key says so in the status bar rather than silently failing, and `wakeart doctor` reports which one it found.

### Scene advance

The carousel can be switched off with `a` (or `--no-carousel`, or `carousel = false`). The scene keeps animating; it just never advances, and the hold progress goes blank. Switching it back on restarts the hold from zero rather than advancing immediately. A held still scene with no motion effects schedules no wake-ups at all.

Each scene holds for `duration` (default 20s), then transitions. A scene's own `hold` beats the configured default; an explicit `--duration` beats the scene, because a duration the user just typed should win over one baked into a file. Pausing stops both the hold timer and frame animation. Manual navigation resets the hold timer.

Transitions are part of the theme, picked at random from an enabled set:

- `cut` — instant
- `dissolve` — characters swap over ~250ms in random order
- `scanwipe` — a bright line sweeps down, replacing rows behind it
- `glitchcut` — 120ms of corruption, then the new scene

### Focus and resize

What happens on `FocusLost` is configurable, because the obvious answer turned out to be wrong. Ambient art often sits on a second monitor while you work in another window, and freezing it the moment the terminal is not frontmost makes it useless for exactly the case it exists for.

- `run` (default) — keep animating. Losing focus does not mean losing visibility.
- `throttle` — drop to 2fps while unfocused. Still alive, nearly free.
- `pause` — suspend the loop entirely: no ticks, no wakeups. For a laptop on battery.

Set with `on_blur` in config or `--on-blur`. Terminals that don't report focus just keep animating; that's the fallback, not a failure.

On resize, scenes re-centre. If a scene is wider or taller than the viewport it is cropped from the centre, and the status bar carries a `⊕` marker. Below 20×10 the app shows a single centred `[ too small ]` and stops animating.

---

## 3. Scene format

A scene is one UTF-8 text file with TOML frontmatter. That's the whole contract.

```
~/.config/wakeart/scenes/my-art.scene
```

```toml
---
name    = "Neon Skyline"
author  = "jb"
palette = "synthwave"      # optional; falls back to global
fps     = 8                # frames per second for this scene
loop    = "pingpong"       # forward | pingpong | once
hold    = "30s"            # override global scene duration
align   = "center"         # center | topleft | bottomleft
effects = ["scanlines"]    # override global effect set
tags    = ["city", "wide"]
---
   ▄▄▄▄   ▄▄▄▄▄▄   ▄▄
  █░░░░█  █░░░░░█  █░█
  ▀▀▀▀▀▀  ▀▀▀▀▀▀▀  ▀▀▀
===
   ▄▄▄▄   ▄▄▄▄▄▄   ▄▄
  █░▒░░█  █░░▒░░█  █░█
  ▀▀▀▀▀▀  ▀▀▀▀▀▀▀  ▀▀▀
```

Rules:

- Frontmatter is optional. A bare `.scene` file with no `---` block is a valid single-frame scene; `name` defaults to a title-cased filename.
- `===` alone on a line separates frames. One frame is fine — it just sits there, which is what most art wants.
- Trailing whitespace is stripped; leading whitespace is significant.
- A file whose frames differ in width is padded to the widest line. No error.
- Files are loaded lazily-ish: metadata is parsed at startup, frame bodies on first display. A hundred scenes must not slow launch.

### Colour

Three ways to colour a scene, in priority order:

1. **Raw ANSI passthrough.** If a frame contains SGR escape sequences, they're honoured verbatim and auto-colouring is skipped. This is what makes the enormous body of existing ANSI art usable — paste it in and it works.
2. **Colour mask.** An optional block after a `~~~` separator, same dimensions as the frame, where each character is a palette index `0`–`9`. Gives precise control for hand-tuned pieces.
3. **Density ramp (default).** Each glyph is scored on ink weight (` .:-=+*#%@` and block-drawing characters have known weights) and mapped across the palette gradient. Costs the author nothing and makes plain ASCII look intentional.

```
   ▄▄▄▄
  █░░░░█
~~~
   1122
  012210
```

### Procedural scenes

A scene file with a `kind` generates its geometry per frame instead of carrying art. The body is empty; the frontmatter is the whole scene.

```toml
---
name  = "Icosahedron"
kind  = "wireframe"        # wireframe | shaded
shape = "icosahedron"
spin  = [0.17, 0.44, 0.09] # radians per second about x, y, z
scale = 0.85               # fraction of the viewport
cull  = false              # drop far-side edges; convex shapes only
fps   = 24
---
```

`wireframe` shapes: `cube`, `tetrahedron`, `octahedron`, `icosahedron`, `torus`, `sphere`, `diamond`, `knot` — the last a (3, 2) torus knot, a single unbroken strand because 3 and 2 are coprime. Edges are drawn with Bresenham, the glyph chosen from the segment's slope (`─ │ ╱ ╲`), and depth mapped onto the gradient so near geometry reads brighter. A z-buffer keeps near edges over far ones.

`shaded` surfaces: `torus`, `sphere`. Each carries an albedo pattern fixed to the surface, without which a rotating sphere is a still image — it is symmetric, so turning it changes nothing you can see. The sphere's longitude term mixes the first and third harmonics; a pure third harmonic is three-fold symmetric and makes the planet look like it turns three times per revolution. These sample the surface densely, keep the nearest sample per cell, and pick a glyph from the dot product of the surface normal with the light — the donut.c approach. The ramp index is also the ink weight, so glyph density and palette gradient agree.

`tunnel` slides concentric rings toward the viewer down a perspective divide, with spokes turning around them — the one scene whose motion is along Z rather than a rotation.

`scope` traces a Lissajous figure the way an oscilloscope does: a phosphor beam runs the curve, bright at the head and fading behind it, while the phase offset walks so the figure folds through itself. `spin` carries the frequency ratio and the phase period, not rotation rates.

`rain` is the glyph cascade. Each column falls a whole number of times over the loop, and the glyph in a cell is a pure function of its position and which fall it belongs to, so the whole thing repeats exactly.

`terrain` is a wireframe landscape sliding toward the viewer, with front-to-back hidden-line removal: a far ridge is drawn only where it rises above everything nearer. Without that the rows cross into noise. The height function is periodic in the scroll coordinate, so a row wrapping from near to far lands on exactly the profile the row ahead of it had.

`gears` draws a train of meshing cogs, out of gear characters. The train is defined as a chain — each cog's bearing from the one before it — so pitch circles are tangent by construction. Angular speed is inversely proportional to radius and neighbours counter-rotate, which is what makes it read as a mechanism rather than three unrelated spinning circles. Each cog is a continuous root circle with teeth spiked outward; modulating the rim radius instead leaves the root arc broken wherever a tooth steps out, which at terminal resolution reads as a dashed circle.

`storm` draws branching lightning by midpoint displacement, halving the sideways spread at each level to get the self-similar kink. A strike's geometry is a pure function of its index, and which strikes are alive follows from the time, so the whole animation is reproducible from `t` alone with no state carried between frames.

### Themed glyph sets

Any procedural scene may replace its glyph set with `glyphs`, a ramp running dim to bright:

```toml
glyphs = "·⌁↯ϟ⚡"     # an icosahedron drawn out of lightning bolts
glyphs = "·◦⊛⚙"      # a torus drawn out of gears
```

Wireframes index the ramp by depth, shaded surfaces by luminance. Both mean the shape is drawn out of the themed characters while the palette still carries the depth.

Glyph width is load-bearing here. `⚡` is a double-width cell and `⚙` is not, so the frame buffer tracks continuation cells: a wide glyph claims the cell to its right, and both the writer and the painter repair pairs broken by a crop, a glitch row-shift, or a narrow glyph overwriting a wide one's second half. Getting this wrong shifts every character to the right of the glyph, and it applies to art read from a file just as much as to generated scenes.

The generator is code; which generator, and its parameters, are data. Adding a variant is a new file, not a rebuild.

Terminal cells are about twice as tall as they are wide, so horizontal distance is doubled in projection or every shape comes out squashed.

### Discovery and precedence

1. Embedded defaults, compiled in via `go:embed scenes/`.
2. `$XDG_CONFIG_HOME/wakeart/scenes/**.scene` (default `~/.config/wakeart/scenes/`), recursive.
3. `--scene-dir <path>`, repeatable.

A user scene whose filename matches an embedded one replaces it — that's how you edit a built-in without forking.

### Validation

`wakeart doctor` reports, per scene: parse errors, dimensions, frame count, whether it fits the current terminal, and which colouring path it takes. Malformed scenes are skipped at runtime with a one-line warning on exit, never a crash mid-carousel.

---

## 4. The cyberpunk layer

The theme lives in two places: palettes and effects. Both are global, both are per-scene overridable, and both are one keypress from off.

### Palettes

Each is a gradient of 8–10 stops in truecolor, with hand-picked 256 and 16-colour approximations rather than automatic quantisation — automatic quantisation of neon gradients looks like mud.

| Name | Feel |
|---|---|
| `neon` | magenta → violet → cyan. The default. |
| `synthwave` | deep purple → hot pink → sunset orange |
| `acid` | black-green → lime → white-yellow |
| `bladerunner` | midnight blue → amber → smoke |
| `ice` | navy → cyan → white |
| `bloodmoon` | maroon → red → orange |
| `mono` | terminal foreground only; the NO_COLOR path |

### Effects

Post-processing applied to the composed frame, after colouring, before paint. Each has three intensities: `off`, `subtle` (default), `heavy`.

- **scanlines** — every second row dimmed 15–30%. Cheap, and does most of the CRT work on its own.
- **flicker** — global brightness jitter, a few percent, on a slow random walk.
- **glitch** — on a Poisson schedule (~1 burst per 12s at subtle), 2–6 rows shift horizontally by a few cells and a scatter of glyphs are replaced from a corruption set (`▓▒░█▄▀╳`) for 2–4 frames.
- **chroma** — a magenta and a cyan ghost of high-contrast rows offset by one cell. Fake RGB split. Expensive-looking, cheap to compute, and the single most "cyberpunk" thing in the list.

Effects are stateless functions over a frame buffer plus an RNG seeded per run, so a given seed replays identically — which is what makes them testable.

`e` cycles the intensity; `E` (or `x`) cycles *which* effect runs, stepping from the configured set through each effect on its own and back again, so a single effect can be seen in isolation without editing config. A cycled effect beats the scene's own list, the way a cycled palette does, and the status bar names each selection as it is picked.

`--no-effects` and `NO_COLOR=1` both drop straight to clean text.

---

## 5. CLI

```
wakeart [flags]              Start the carousel

Flags:
  --scene <name>             Start on a specific scene
  --only <tag>               Restrict the deck to a tag (repeatable)
  --shuffle                  Random order
  --duration <dur>           Seconds per scene, overriding each scene's own hold
  --speed <float>            Global animation multiplier (default 1.0)
  --palette <name>           Override palette for all scenes
  --effects <list|none>      Override effect set
  --scene-dir <path>         Additional scene directory (repeatable)
  --once                     Print one frame to stdout and exit
  --seed <int>               Deterministic run
  --no-color                 Monochrome
  --on-blur <mode>           When the terminal loses focus: run | throttle | pause
  --config <path>            Alternate config file

Commands:
  wakeart list               Table of scenes: name, source, frames, size, tags
  wakeart new <name>         Scaffold a scene file in the user dir and open $EDITOR
  wakeart add <file>         Copy a file into the user scene dir, inferring metadata
  wakeart show <name>        Render one scene once, non-interactively
  wakeart doctor             Terminal capabilities + scene validation report
```

`--once` is the shell-startup path: `wakeart --once --scene cube` in your `.zshrc` prints a single frame as a banner and exits. It writes to stdout, respects pipes, and emits no escape sequences when not a TTY.

### Config

`~/.config/wakeart/config.toml`. Flags override file; file overrides defaults.

```toml
duration  = "20s"
shuffle   = true
palette   = "neon"
effects   = ["scanlines", "glitch", "chroma"]
intensity = "subtle"
speed     = 1.0
status_bar = true
carousel   = true
awake      = false
on_blur    = "run"
transitions = ["cut", "scanwipe", "glitchcut"]

[keys]
next = ["n", "right", "l"]
quit = ["q", "esc", "ctrl+c"]
```

---

## 6. Architecture

```
cmd/wakeart/main.go          flag parsing, config merge, wiring
internal/scene/           parse, validate, registry, lazy loading
internal/proc/            procedural scenes: meshes, projection, rasterising
internal/render/          frame buffer, ramp colouring, fit/crop/centre, transitions
internal/effect/          scanlines, flicker, glitch, chroma
internal/awake/           sleep inhibitor: caffeinate / systemd-inhibit
internal/palette/         palette definitions, capability degradation
internal/tui/             bubbletea model, keymap, status bar, help overlay
internal/config/          TOML load, defaults, XDG paths
scenes/                   embedded default scenes
```

Dependencies, deliberately few: `bubbletea`, `lipgloss`, `bubbles/key`, `go-toml/v2`. Nothing else.

### Core types

```go
type Meta struct {
    Name, Author, Palette string
    FPS                   float64
    Loop                  LoopMode
    Hold                  time.Duration
    Align                 Align
    Effects               []string
    Tags                  []string
    Source                string // path or "embedded:<name>"
}

type Frame struct {
    Cells [][]Cell // row-major, rectangular, pre-padded
    W, H  int
}

type Cell struct {
    R     rune
    Style Style // fg/bg/attrs, or Raw for passthrough spans
}

// A scene is either art read from a file or a procedural renderer.
// Both end up as an ordinary Frame, so palettes, effects and
// transitions apply to them identically.
type Renderer interface {
    Frame(t time.Duration, w, h int) Frame
    Describe() string
}
```

### Render loop

One `tea.Tick` at the active scene's FPS — not a fixed 60Hz clock. A one-frame scene ticks only for its hold timer and glitch schedule; a static scene with effects off ticks once and then sits at zero wakeups until input.

Coloured frames are cached by `(scene, frameIndex, width, palette)`. Effects run on a copy of the cached buffer, so the expensive work happens once per scene and the per-tick cost is a memcpy plus a row-wise transform. Cache is bounded (LRU, ~64 entries) so a big deck can't grow memory without limit.

### Budgets

| Metric | Target |
|---|---|
| Idle CPU, default settings, focused | < 1% |
| Idle CPU, unfocused, `on_blur = "pause"` | 0% |
| RSS | < 30MB with 100 scenes loaded |
| Startup to first frame | < 50ms |
| Binary size | < 10MB |

These belong in a benchmark, not just in a document. `make bench` measures startup and per-frame render time; CI fails on regression beyond 20%.

### Terminal capability

Detect once at startup: `COLORTERM=truecolor|24bit` → truecolor; `TERM` containing `256color` → 256; `NO_COLOR` set or `TERM=dumb` → mono. `--no-color` forces mono. `wakeart doctor` prints what it detected and why, because this is the thing that will generate bug reports.

### Testing

- Golden-file tests: render each scene at 80×24 and 120×40, strip colour, compare to a checked-in fixture.
- Palette tests: assert every palette's 16-colour approximation has a minimum contrast against the terminal background.
- Effect tests: fixed seed, assert byte-identical output across runs.
- Parser tests: malformed frontmatter, ragged frames, empty files, mask/frame dimension mismatch — every one must degrade, not panic.

---

## 7. Starter deck

Fifteen scenes ship embedded, all generated per frame. They set the taste, so they matter more than the count:

**Rotating solids** — Wireframe (cube, ice), Icosahedron (acid), Globe (sphere, far side culled, bladerunner), Prism (diamond drawn in lightning glyphs, ice), Torus Knot (bloodmoon), Donut (lit torus, synthwave), Planet (lit sphere with banded albedo, bladerunner), Reactor (torus drawn out of gears, acid), Arc Net (icosahedron drawn out of lightning, synthwave).

**Mechanisms and weather** — Gearworks (three meshing cogs in `⚙`, bladerunner), Storm (branching lightning in `⚡`, ice).

**Other motion** — Tunnel (rings rushing past, neon), Ridge (scrolling wireframe landscape, synthwave), Scope (Lissajous trace on a phosphor beam, acid), Cascade (falling glyph columns, acid).

Every one declares an exact loop period, which is what lets a capture cut without a seam.

No art files ship. The `.scene` format, colour masks and ANSI passthrough remain fully supported for scenes a user adds, and two fixture files under `internal/tui/testdata/scenes` keep that path covered by the golden tests.

## 8. Milestones

**M1 — Walking skeleton.** Parser, registry, static render, centring, resize, quit-clean. One scene, no colour. Proves the loop and the teardown.

**M2 — Carousel.** Multi-frame animation, hold timer, navigation keys, shuffle, status bar, pause. Ten scenes in plain text.

**M3 — The look.** Palettes, density ramp, colour masks, ANSI passthrough, effects, transitions, capability degradation. This is the milestone the project exists for.

**M4 — Authorability.** User scene dir, `new` / `add` / `list` / `doctor`, config file, remappable keys, `--once`.

**M5 — Ship.** Focus suspension, perf benchmarks, golden tests, README with a recorded GIF, `go install` + Homebrew tap + GoReleaser for darwin/linux, arm64/amd64.

---

## 9. Open questions

- **More procedural kinds.** The renderer interface takes any generator, so a starfield, a plasma field or a matrix cascade are all a new file in `internal/proc` plus a name in the build switch. Proposal: add on appetite; four kinds ship.

- **Shipping art again.** The deck is generated-only, which keeps the binary honest about what it is. Hand-drawn scenes could return as an optional pack. Proposal: leave it; a directory of files is already a shareable pack.
- **Fit-to-width scaling.** Wide art on a narrow pane could downscale by sampling. It always looks worse than cropping. Proposal: crop by default, ship `f` as an escape hatch, revisit if it annoys.
- **A scene pack format.** A `.tar.gz` of scenes plus a manifest would make sharing collections easy, but it invites a registry, and a registry invites network access. Proposal: out of scope for v1; a directory of files is already a shareable pack.
- **Reading from stdin.** `figlet HELLO | wakeart --stdin` is a tempting one-liner. Cheap to add, slightly off-mission. Proposal: after M5, if wanted.
- **Windows.** Bubble Tea works on Windows Terminal, but the block-drawing glyphs and focus reporting need verification. Proposal: build for it, don't claim support until someone tests it.
