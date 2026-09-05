package scene

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/verdantran/wakeart/internal/proc"
)

const Ext = ".scene"

// MaxBytes bounds what one scene file may cost. A frame allocates a cell per
// glyph, tens of bytes each, so a file far past this size turns into gigabytes
// of buffer. Real art is a few kilobytes.
const MaxBytes = 8 << 20

type Registry struct {
	Scenes []*Scene
	Errs   []error
}

func (r *Registry) Len() int { return len(r.Scenes) }

// SlugOf is the filename-derived handle a scene answers to on the command line.
func SlugOf(s *Scene) string {
	src := s.Meta.Source
	if rest, ok := strings.CutPrefix(src, "embedded:"); ok {
		return rest
	}
	return strings.TrimSuffix(filepath.Base(src), Ext)
}

func (r *Registry) Find(name string) (*Scene, int) {
	want := strings.ToLower(name)
	for i, s := range r.Scenes {
		if strings.ToLower(s.Meta.Name) == want {
			return s, i
		}
		if strings.ToLower(SlugOf(s)) == want {
			return s, i
		}
	}
	return nil, -1
}

// FilterTags keeps scenes carrying any of the given tags.
func (r *Registry) FilterTags(tags []string) {
	if len(tags) == 0 {
		return
	}
	want := map[string]bool{}
	for _, t := range tags {
		want[strings.ToLower(t)] = true
	}
	var keep []*Scene
	for _, s := range r.Scenes {
		for _, t := range s.Meta.Tags {
			if want[strings.ToLower(t)] {
				keep = append(keep, s)
				break
			}
		}
	}
	r.Scenes = keep
}

// Load reads embedded defaults then each directory in turn. A later scene
// whose filename matches an earlier one replaces it, which is how a user edits
// a built-in without forking.
func Load(embedded fs.FS, dirs []string) *Registry {
	r := &Registry{}
	byKey := map[string]int{}

	add := func(key string, s *Scene) {
		if i, ok := byKey[key]; ok {
			r.Scenes[i] = s
			return
		}
		byKey[key] = len(r.Scenes)
		r.Scenes = append(r.Scenes, s)
	}

	if embedded != nil {
		entries, _ := fs.ReadDir(embedded, ".")
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), Ext) {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, n := range names {
			data, err := fs.ReadFile(embedded, n)
			if err != nil {
				r.Errs = append(r.Errs, fmt.Errorf("embedded %s: %w", n, err))
				continue
			}
			s, err := Parse("embedded:"+strings.TrimSuffix(n, Ext), data)
			if err != nil {
				// A kind this machine cannot run is not a broken scene; the
				// deck simply does not offer it here.
				if !errors.Is(err, proc.ErrKindUnavailable) {
					r.Errs = append(r.Errs, fmt.Errorf("embedded %s: %w", n, err))
				}
				continue
			}
			add(strings.TrimSuffix(n, Ext), s)
		}
	}

	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		var found []string
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && strings.HasSuffix(path, Ext) {
				found = append(found, path)
			}
			return nil
		})
		sort.Strings(found)
		for _, path := range found {
			if fi, err := os.Stat(path); err == nil && fi.Size() > MaxBytes {
				r.Errs = append(r.Errs, fmt.Errorf("%s: %d bytes exceeds the %d-byte limit", path, fi.Size(), MaxBytes))
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				r.Errs = append(r.Errs, fmt.Errorf("%s: %w", path, err))
				continue
			}
			s, err := Parse(path, data)
			if err != nil {
				if !errors.Is(err, proc.ErrKindUnavailable) {
					r.Errs = append(r.Errs, fmt.Errorf("%s: %w", path, err))
				}
				continue
			}
			add(strings.TrimSuffix(filepath.Base(path), Ext), s)
		}
	}
	return r
}
