package ytmusic

// YouTube's InnerTube responses are deeply nested maps where every
// concrete piece of data is wrapped in a "renderer" object. The
// shape changes silently; today's musicResponsiveListItemRenderer has
// `flexColumns`, tomorrow it might have `fixedColumns` for certain
// items. These helpers tolerate missing keys rather than asserting
// types and panicking, so a YT response-shape tweak that drops one
// field doesn't take the whole import down.

// walk returns the value at a nested path of string keys / int indices.
// A missing key, nil value, or wrong-type intermediate node yields nil.
// This is the only way to navigate InnerTube safely without dozens of
// typed-or-false guards at each call site.
func walk(v any, keys ...any) any {
	for _, k := range keys {
		if v == nil {
			return nil
		}
		switch kk := k.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				return nil
			}
			v = m[kk]
		case int:
			s, ok := v.([]any)
			if !ok || kk < 0 || kk >= len(s) {
				return nil
			}
			v = s[kk]
		default:
			return nil
		}
	}
	return v
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// runsText concatenates the `text` fields of a `runs` array — YT
// represents "Artist A & Artist B · Album" as a list of 3+ runs
// ({"text": "Artist A"}, {"text": " & "}, {"text": "Artist B"}).
// For names and titles we usually want the whole joined string.
func runsText(runs any) string {
	var out string
	for _, r := range asSlice(runs) {
		out += asString(walk(r, "text"))
	}
	return out
}

// firstRunText returns the text of the first run — useful when YT
// encodes an entity as a single navigable run (a playlist title, a
// track name) and the remaining runs are separator / metadata noise.
func firstRunText(runs any) string {
	for _, r := range asSlice(runs) {
		if t := asString(walk(r, "text")); t != "" {
			return t
		}
	}
	return ""
}

// navBrowseID extracts `navigationEndpoint.browseEndpoint.browseId`
// from a run. Used to find playlist / album / artist IDs associated
// with a clickable piece of text.
func navBrowseID(run any) string {
	return asString(walk(run,
		"navigationEndpoint", "browseEndpoint", "browseId"))
}

// findShelfItems traverses the standard library-page shape and
// returns the list of musicResponsiveListItemRenderer / equivalent
// entries from the first musicShelfRenderer it encounters.
//
// Library pages pin the shelf at:
//
//	contents.singleColumnBrowseResultsRenderer.tabs[0].tabRenderer.
//	  content.sectionListRenderer.contents[N].
//	    {gridRenderer|musicShelfRenderer|musicCarouselShelfRenderer}.
//	      {items|contents}
//
// We search for the first renderer among those variants because which
// one YT uses depends on whether the user has many items (shelf) or
// few (grid), and whether it's a horizontal carousel (albums page).
func findShelfItems(resp map[string]any) []any {
	sections := asSlice(walk(resp,
		"contents",
		"singleColumnBrowseResultsRenderer",
		"tabs", 0,
		"tabRenderer",
		"content",
		"sectionListRenderer",
		"contents"))
	for _, section := range sections {
		if items := asSlice(walk(section, "musicShelfRenderer", "contents")); items != nil {
			return items
		}
		if items := asSlice(walk(section, "gridRenderer", "items")); items != nil {
			return items
		}
		if items := asSlice(walk(section, "musicCarouselShelfRenderer", "contents")); items != nil {
			return items
		}
	}
	return nil
}
