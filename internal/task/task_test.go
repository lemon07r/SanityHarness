package task

import "testing"

func TestParseTaskID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		ok   bool
		lang Language
		slug string
	}{
		{name: "canonical", in: "go/bank-account", ok: true, lang: Go, slug: "bank-account"},
		{name: "canonical whitespace", in: "  rust/regex-lite  ", ok: true, lang: Rust, slug: "regex-lite"},
		{name: "missing slug", in: "go/", ok: false},
		{name: "missing lang", in: "/bank-account", ok: false},
		{name: "unknown lang", in: "python/foo", ok: false},
		{name: "too many slashes", in: "go/a/b", ok: false},
		{name: "no slash", in: "bank-account", ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			lang, slug, ok := ParseTaskID(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok=%v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if lang != tc.lang {
				t.Fatalf("lang=%q, want %q", lang, tc.lang)
			}
			if slug != tc.slug {
				t.Fatalf("slug=%q, want %q", slug, tc.slug)
			}
		})
	}
}

func TestResolveRef(t *testing.T) {
	t.Parallel()

	tasks := []*Task{
		{Slug: "react", Language: Go},
		{Slug: "react", Language: TypeScript},
		{Slug: "bank-account", Language: Go},
	}

	t.Run("canonical id", func(t *testing.T) {
		t.Parallel()

		got, err := ResolveRef(tasks, "go/react")
		if err != nil {
			t.Fatalf("ResolveRef error: %v", err)
		}
		if got.Language != Go || got.Slug != "react" {
			t.Fatalf("got %s, want go/react", got.ID())
		}
	})

	t.Run("bare slug unambiguous", func(t *testing.T) {
		t.Parallel()

		got, err := ResolveRef(tasks, "bank-account")
		if err != nil {
			t.Fatalf("ResolveRef error: %v", err)
		}
		if got.Language != Go || got.Slug != "bank-account" {
			t.Fatalf("got %s, want go/bank-account", got.ID())
		}
	})

	t.Run("bare slug ambiguous", func(t *testing.T) {
		t.Parallel()

		_, err := ResolveRef(tasks, "react")
		if err == nil {
			t.Fatalf("expected error")
		}
		want := "task slug \"react\" is ambiguous; use one of: go/react, typescript/react"
		if err.Error() != want {
			t.Fatalf("error=%q, want %q", err.Error(), want)
		}
	})
}
