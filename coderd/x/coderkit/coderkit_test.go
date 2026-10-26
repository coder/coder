package coderkit_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/coderkit"
	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestPack(t *testing.T) {
	t.Parallel()

	t.Run("PageWithoutKitIsUnchanged", func(t *testing.T) {
		t.Parallel()
		page := []byte(`<!doctype html><link rel="stylesheet" href="style.css"><h1>hi</h1>`)
		require.Equal(t, page, coderkit.Pack(page, coderkit.ThemeDark))
	})

	t.Run("InlinesKitAndScript", func(t *testing.T) {
		t.Parallel()
		page := `<!doctype html><html><head><link rel="stylesheet" href="./kit/coder-kit.css">` +
			`<script src="coder-kit.js" defer></script></head><body class="ck-page"><h1 class="ck-h1">Report</h1></body></html>`
		packed := string(coderkit.Pack([]byte(page), coderkit.ThemeNone))

		require.True(t, strings.HasPrefix(packed, "<!doctype html><html><head><style data-coder-kit>\n"))
		require.True(t, strings.HasSuffix(packed, `</head><body class="ck-page"><h1 class="ck-h1">Report</h1></body></html>`))
		require.NotContains(t, packed, `href="./kit/coder-kit.css"`)
		require.NotContains(t, packed, `src="coder-kit.js"`)
		require.NotContains(t, packed, `setAttribute("data-theme"`, "no theme leaves it to the OS")
		// The imports and the fonts are inline, because the frame has no
		// network and no same-origin access.
		require.NotContains(t, packed, "@import")
		require.NotContains(t, packed, `url("fonts/`)
		require.Equal(t, 2, strings.Count(packed, `url("data:font/woff2;base64,`))
		require.Contains(t, packed, ".ck-btn", "components.css is inline")
		require.Contains(t, packed, ".ck-icon-play", "icons.css is inline")
		require.Contains(t, packed, "window.CoderKit", "coder-kit.js is inline")
		// The style and script bodies must not close their own tags.
		css := packed[strings.Index(packed, "<style data-coder-kit>")+len("<style data-coder-kit>"):]
		require.Equal(t, 1, strings.Count(strings.ToLower(css), "</style"))
		js := packed[strings.Index(packed, "<script data-coder-kit>")+len("<script data-coder-kit>"):]
		require.Equal(t, 1, strings.Count(strings.ToLower(js), "</script"))
	})

	t.Run("ThemeSetsRootUnlessPageDoes", func(t *testing.T) {
		t.Parallel()
		page := []byte(`<link rel="stylesheet" href="coder-kit.css"><body class="ck-page"></body>`)
		packed := string(coderkit.Pack(page, coderkit.ThemeDark))
		require.True(t, strings.HasPrefix(packed, `<script data-coder-kit>`))
		require.Contains(t, packed, `if(!r.hasAttribute("data-theme"))r.setAttribute("data-theme","dark")`)
		require.Less(t, strings.Index(packed, "<script data-coder-kit>"), strings.Index(packed, "<style data-coder-kit>"),
			"the theme is set before the stylesheet applies")
	})
}

func TestParseTheme(t *testing.T) {
	t.Parallel()
	require.Equal(t, coderkit.ThemeLight, coderkit.ParseTheme("light"))
	require.Equal(t, coderkit.ThemeDark, coderkit.ParseTheme("dark"))
	for _, value := range []string{"", "auto", "Dark", `dark")</script>`} {
		require.Equal(t, coderkit.ThemeNone, coderkit.ParseTheme(value), value)
	}
}

func TestLinksKit(t *testing.T) {
	t.Parallel()
	require.True(t, coderkit.LinksKit([]byte(`<LINK REL="stylesheet" HREF='/a/coder-kit.css'>`)))
	require.False(t, coderkit.LinksKit([]byte(`<link rel="stylesheet" href="CODER-KIT.CSS">`)), "the file name is case-sensitive")
	require.False(t, coderkit.LinksKit([]byte(`<link rel="stylesheet" href="coder-kit.css.map">`)))
	require.False(t, coderkit.LinksKit([]byte(`<p>coder-kit.css</p>`)))
}

func TestUnknownClasses(t *testing.T) {
	t.Parallel()
	page := `<div class="ck-panel other ck-panl"><i class='ck-icon ck-icon-play ck-icon-rocket'></i><b class="ck-panl"></b></div>` +
		`<script>el.innerHTML = '<span class="ck-built-at-run-time"></span>';</script>`
	require.Equal(t, []string{"ck-icon-rocket", "ck-panl"}, coderkit.UnknownClasses([]byte(page)))
	require.Empty(t, coderkit.UnknownClasses([]byte(`<p class="lead">no kit</p>`)))
	require.Empty(t, coderkit.UnknownClasses([]byte(`<p data-class="ck-not-a-class">x</p>`)))

	var many strings.Builder
	for i := range 50 {
		_, _ = fmt.Fprintf(&many, `<b class="ck-made-up-%02d"></b>`, i)
	}
	unknown := coderkit.UnknownClasses([]byte(many.String()))
	require.Len(t, unknown, 20, "the list is bounded")
	require.Equal(t, "ck-made-up-00", unknown[0])
}

func TestSkill(t *testing.T) {
	t.Parallel()
	skill := coderkit.Skill()
	require.Equal(t, "coder-kit", skill.Name)
	require.True(t, workspacesdk.SkillNamePattern.MatchString(skill.Name))
	require.Equal(t, skills.SourceBuiltin, skill.Source)
	require.NotEmpty(t, skill.Description)
	require.LessOrEqual(t, len(skill.Description), skills.MaxPersonalSkillDescriptionBytes)
	require.Contains(t, skill.Body, `<link rel="stylesheet" href="coder-kit.css">`)

	// Every class and icon that the skill teaches must exist in the kit, so
	// an update of kit/ cannot leave the skill pointing at removed parts.
	var classes []string
	// The prefix skips attributes such as data-ck-open.
	for _, m := range regexp.MustCompile(`(?:^|[^a-z0-9-])(ck-[a-z0-9]+(?:-[a-z0-9]+)*)`).FindAllStringSubmatch(skill.Body, -1) {
		classes = append(classes, m[1])
	}
	require.Greater(t, len(classes), 50)
	iconLine := regexp.MustCompile("(?m)^The icons: (.*)$").FindStringSubmatch(skill.Body)
	require.NotNil(t, iconLine, "the skill lists the icons")
	icons := regexp.MustCompile("`([a-z0-9-]+)`").FindAllStringSubmatch(iconLine[1], -1)
	require.Greater(t, len(icons), 50)
	for _, icon := range icons {
		classes = append(classes, "ck-icon-"+icon[1])
	}
	require.Empty(t, coderkit.UnknownClasses([]byte(`<i class="`+strings.Join(classes, " ")+`"></i>`)))
}
