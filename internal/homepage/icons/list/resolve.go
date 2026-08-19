package iconlist

import (
	"strings"

	"github.com/yusing/godoxy/internal/homepage/icons"
)

var nameSanitizer = strings.NewReplacer(
	"_", "-",
	" ", "-",
	"(", "",
	")", "",
)

func sanitizeRef(name string) string {
	return strings.ToLower(nameSanitizer.Replace(name))
}

// ExpandRefs returns sanitized lookup names for icon/category matching.
// Names combine the app with a role or a qualifier of the user's own, so each
// trailing segment is dropped in turn and the shorter names are offered too:
// "immich-server" and "sonarr-tv" also yield "immich" and "sonarr".
func ExpandRefs(refs []string) []string {
	seen := make(map[string]struct{}, len(refs)*2)
	out := make([]string, 0, len(refs)*2)
	add := func(ref string) {
		if ref == "" {
			return
		}
		if _, ok := seen[ref]; ok {
			return
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	for _, ref := range refs {
		ref = sanitizeRef(ref)
		add(ref)
		for {
			dash := strings.LastIndexByte(ref, '-')
			if dash <= 0 {
				break
			}
			ref = ref[:dash]
			add(ref)
		}
	}
	return out
}

func preferredFileType(meta *icons.Meta) string {
	switch {
	case meta.SVG:
		return "svg"
	case meta.WebP:
		return "webp"
	case meta.PNG:
		return "png"
	default:
		return ""
	}
}

// LookupURL returns a catalog icon for an exact reference, preferring selfh.st.
func LookupURL(ref string) *icons.URL {
	ref = sanitizeRef(ref)
	if ref == "" {
		return nil
	}
	iconsMap := ListAvailableIcons()
	for _, source := range []icons.Source{icons.SourceSelfhSt, icons.SourceWalkXCode} {
		meta, ok := iconsMap.Load(icons.NewKey(source, ref))
		if !ok {
			continue
		}
		ft := preferredFileType(meta)
		if ft == "" {
			continue
		}
		return icons.NewURL(source, ref, ft)
	}
	return nil
}

// Resolve finds catalog metadata and/or an icon URL from route references.
func Resolve(refs []string) (u *icons.URL, meta HomepageMeta, ok bool) {
	var foundMeta bool
	for _, ref := range ExpandRefs(refs) {
		if !foundMeta {
			if m, found := GetMetadata(ref); found {
				meta = m
				foundMeta = true
			}
		}
		if u == nil {
			u = LookupURL(ref)
		}
		if foundMeta && u != nil {
			break
		}
	}
	return u, meta, foundMeta || u != nil
}
