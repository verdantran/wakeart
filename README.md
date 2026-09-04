# wakeart

ASCII art on a loop, for a spare terminal pane.

Leave it running in a tmux split while you work. Eight generated scenes —
tumbling wireframes, a lit donut, a gear train, branching lightning — coloured
with neon gradients and lightly corrupted with scanlines and glitch bursts.
Adding your own art is dropping a text file in a folder.

```
$ wakeart
```

`q` quits and leaves your terminal exactly as it was.

## Install

```
go install github.com/verdantran/wakeart/cmd/wakeart@latest
```

Or build from source:

```
make build && ./wakeart
```

## Keys

| Key | Action |
|---|---|
| `space` | Pause / resume |
| `n`, `→`, `l` | Next scene |
| `p`, `←`, `h` | Previous scene |
| `a` | Toggle the carousel — hold on one scene, or auto-advance |
| `k` | Toggle keep-awake — stop the machine sleeping |
| `s` | Toggle shuffle |
| `c` | Cycle palette |
| `e` | Cycle effect intensity — off / subtle / heavy |
| `+` / `-` | Speed up / slow down |
| `f` | Toggle fit-to-viewport scaling |
| `i` | Toggle status bar |
| `?` | Help |
| `q`, `esc`, `ctrl+c` | Quit |

Every binding is remappable. The help overlay is generated from the keymap, so
it cannot drift from what the keys actually do.

## What ships

Fifteen scenes, all generated per frame:

| Scene | Kind | Palette |
|---|---|---|
| Wireframe | tumbling cube | ice |
| Icosahedron | slow tumble | acid |
| Globe | wire globe, far side culled | bladerunner |
| Prism | diamond drawn in lightning glyphs | ice |
| Torus Knot | a (3, 2) knot, one unbroken strand | bloodmoon |
| Donut | lit torus | synthwave |
| Planet | lit sphere, banded albedo | bladerunner |
| Reactor | the torus drawn out of gears | acid |
| Arc Net | an icosahedron drawn out of lightning | synthwave |
| Gearworks | three meshing cogs in `⚙` | bladerunner |
| Storm | branching lightning in `⚡` | ice |
| Tunnel | rings rushing past | neon |
| Ridge | a scrolling wireframe landscape | synthwave |
| Scope | a Lissajous trace on a phosphor beam | acid |
| Cascade | falling glyph columns | acid |

No art files ship, but the file format below is fully supported for scenes you
add yourself.

## Adding a scene

A scene is one UTF-8 text file in `~/.config/wakeart/scenes/`. Frontmatter is
optional — a bare text file is a valid single-frame scene.

```
$ wakeart new my-scene
```

```toml
---
name    = "Neon Skyline"
palette = "synthwave"      # optional; falls back to the global palette
fps     = 8
loop    = "pingpong"       # forward | pingpong | once
hold    = "30s"            # override the global scene duration
align   = "center"         # center | topleft | bottomleft
effects = ["scanlines"]    # override the global effect set
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

`===` alone on a line separates frames. A file whose lines differ in width is
padded to the widest. A user scene whose filename matches a built-in replaces
it, which is how you edit one without forking.

Already have art from somewhere? `wakeart add path/to/art.txt` copies it in and
infers the metadata.

### Rotating shapes

A scene file with a `kind` generates its geometry every frame instead of
carrying art. The body is empty — the frontmatter is the whole scene.

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

Eight kinds:

- `wireframe` — `cube`, `tetrahedron`, `octahedron`, `icosahedron`, `torus`,
  `sphere`, `diamond`, `knot`. The glyph comes from each segment's slope
  (`─ │ ╱ ╲`) and depth maps onto the palette, so near edges read brighter.
- `shaded` — `torus` (the donut) and `sphere`, lit per sample and drawn through
  a z-buffer, the way donut.c does it.
- `gears` — a train of meshing cogs drawn out of `⚙`. Neighbours counter-rotate
  and small cogs turn faster, so it reads as one mechanism.
- `storm` — branching lightning drawn out of `⚡`, struck by midpoint
  displacement and fading through `ϟ ↯ ⌁` as it dies.
- `tunnel` — rings sliding toward you down a perspective divide. The one scene
  whose motion is along Z rather than a rotation.
- `terrain` — a wireframe landscape scrolling toward the viewer, with
  front-to-back hidden-line removal so far ridges stay behind near ones.
- `scope` — a Lissajous figure traced by a phosphor beam. Here `spin` carries
  the frequency ratio and phase period rather than rotation rates.
- `rain` — a cascade of glyph columns, each falling a whole number of times per
  loop.

Every kind declares an exact loop period, so a scene always returns to the
frame it started on.

### Themed characters

Any procedural scene can be drawn out of a different set of characters. `glyphs`
is a ramp running dim to bright — wireframes index it by depth, shaded surfaces
by luminance:

```toml
glyphs = "·⌁↯ϟ⚡"      # an icosahedron built from lightning bolts
glyphs = "·◦⊛⚙"       # a torus built from gears
```

Double-width glyphs are handled properly, so `⚡` in a scene file — or in your
own ASCII art — will not shift everything to the right of it.

The generator is code; which generator, and its parameters, are data. A new
variant is a new file, not a rebuild — copy one, change the `shape` and `spin`,
and it is in the deck.

### Colour

Three ways to colour a scene, in priority order:

1. **Raw ANSI passthrough.** A frame containing SGR escape sequences keeps them
   verbatim. Paste in existing ANSI art and it works.
2. **Colour mask.** A block after a `~~~` separator, the same shape as the
   frame, where each character is a palette index `0`–`9`.
3. **Density ramp** (default). Each glyph is scored on ink weight and mapped
   across the palette gradient. Costs you nothing and makes plain ASCII look
   deliberate.

```
   ▄▄▄▄
  █░░░░█
~~~
   1122
  012210
```

## Palettes and effects

Palettes: `neon`, `synthwave`, `acid`, `bladerunner`, `ice`, `bloodmoon`, `mono`.

Effects: `scanlines`, `flicker`, `glitch`, `chroma` — each at `off`, `subtle`
or `heavy`. They are post-processing over the composed frame, so scene authors
write art, not aesthetics, and the whole look is one keypress from off.

Truecolor is the good path. 256-colour and 16-colour terminals get hand-picked
approximations rather than automatic quantisation, which turns neon gradients
to mud. `NO_COLOR` and `TERM=dumb` drop to clean monochrome text.

## Command line

```
wakeart [flags]              Start the carousel

  --scene <name>             Start on a specific scene
  --only <tag>               Restrict the deck to a tag (repeatable)
  --shuffle                  Random order
  --duration <dur>           Seconds per scene, overrides each scene's hold
  --speed <float>            Global animation multiplier
  --palette <name>           Override palette for all scenes
  --effects <list|none>      Override effect set
  --intensity <level>        off | subtle | heavy
  --scene-dir <path>         Additional scene directory (repeatable)
  --once                     Print one frame to stdout and exit
  --seed <int>               Deterministic run
  --no-carousel              Start held on one scene instead of auto-advancing
  --awake, --caffeinate      Stop the system sleeping while wakeart runs
  --no-color                 Monochrome
  --on-blur <mode>           On losing focus: run | throttle | pause
  --config <path>            Alternate config file

wakeart list                 Table of scenes
wakeart new <name>           Scaffold a scene and open $EDITOR
wakeart add <file>           Copy a file into the user scene dir
wakeart show <name>          Render one scene once, non-interactively
wakeart doctor               Terminal capabilities + scene validation
```

`--once` is the shell-startup path. It writes to stdout, respects pipes, and
emits no escape sequences when it isn't talking to a terminal:

```sh
# in .zshrc
wakeart --once --scene cube
```

## Config

`~/.config/wakeart/config.toml`, honouring `XDG_CONFIG_HOME`. Flags override the
file; the file overrides the defaults.

```toml
duration    = "20s"
shuffle     = true
palette     = "neon"
effects     = ["scanlines", "glitch", "chroma"]
intensity   = "subtle"
speed       = 1.0
status_bar  = true
carousel    = true           # false starts held on one scene
awake       = false          # true starts with sleep held off
on_blur     = "run"          # run | throttle | pause
transitions = ["cut", "scanwipe", "glitchcut"]

[keys]
next = ["n", "right", "l"]
quit = ["q", "esc", "ctrl+c"]
```

## Holding on one scene

`a` stops the carousel where it is. The scene keeps animating; it just never
advances, and the hold progress in the status bar goes blank behind a `⊘`. Press
`a` again and the clock restarts from zero rather than jumping to the next
scene. `--no-carousel` starts that way, and `carousel = false` makes it the
default.

Held on a still scene with nothing else moving, wakeart stops scheduling wake-ups
altogether — there is nothing left to redraw.

## Keeping the machine awake

`k` runs the system's own sleep inhibitor for as long as you want it, and `☼`
appears in the status bar while it is on. On macOS that is `caffeinate -d -i`,
which holds off both display and idle sleep; on Linux it is `systemd-inhibit`.
Elsewhere the key says so rather than silently doing nothing, and `wakeart
doctor` reports which inhibitor it found.

The helper is started with `-w <wakeart's pid>`, so it dies with wakeart even if
wakeart is killed outright — a crash cannot leave your machine awake.

`--awake` (or `--caffeinate`) starts with it on; `awake = true` makes that the
default.

## Losing focus

By default the carousel keeps playing when the terminal is not the frontmost
window — ambient art often sits on a second monitor while you work elsewhere,
and freezing it there defeats the point. Two quieter modes are available:

```
wakeart --on-blur throttle   # drop to 2fps while unfocused
wakeart --on-blur pause      # suspend entirely, zero wakeups
```

## What it costs

Idle cost is the point — a toy that burns a fan gets closed and never
reopened. Measured on an M3 at 120×40 with effects on:

| | Measured | Budget |
|---|---|---|
| Frame from file art (compose, effects, paint) | 34µs | — |
| Frame, rotating wireframe | 14µs | — |
| Frame, lightning storm | 15µs | — |
| Frame, gear train | 69µs | — |
| Frame, shaded donut (the most expensive scene) | 220µs | — |
| CPU, file art at 12fps | ~0.04% | < 1% |
| CPU, shaded donut at 24fps | ~0.53% | < 1% |
| CPU unfocused, `--on-blur pause` | 0% | 0% |
| Startup (load deck, parse, first frame) | 19ms | < 50ms |
| Peak RSS | 6.9MB | < 30MB |
| Binary | 3.9MB | < 10MB |

Ticks run at the active scene's own rate rather than a fixed clock, and a still
scene with no motion effects sleeps until its hold expires instead of ticking
for nothing.

`make bench-check` fails the build on a regression beyond 20% against the
recorded baseline.

## Development

```
make test      # unit, golden and determinism tests
make bench     # render loop benchmarks
make golden    # re-record golden files after a deliberate layout change
make vet
```

Effects are seeded per run, so a given `--seed` replays identically. That is
what makes them testable.
