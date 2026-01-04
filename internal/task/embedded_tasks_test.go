package task

import (
	"testing"

	embeddedtasks "github.com/lemon07r/sanityharness/tasks"
)

func TestEmbeddedTasksLoadAndFilesExist(t *testing.T) {
	t.Parallel()

	loader := NewLoader(embeddedtasks.FS, "")
	tasks, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll error: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatalf("expected embedded tasks")
	}

	for _, tt := range tasks {
		t.Run(tt.ID(), func(t *testing.T) {
			t.Parallel()

			validateEmbeddedTaskMetadata(t, tt)
			validateEmbeddedTaskFiles(t, loader, tt)
		})
	}
}

func validateEmbeddedTaskMetadata(t *testing.T, tt *Task) {
	if tt.Name == "" {
		t.Fatalf("missing name")
	}
	if tt.Description == "" {
		t.Fatalf("missing description")
	}
	if tt.Difficulty == "" {
		t.Fatalf("missing difficulty")
	}
	if tt.Tier != "core" && tt.Tier != "extended" {
		t.Fatalf("invalid tier %q", tt.Tier)
	}
	if len(tt.Files.Stub) == 0 {
		t.Fatalf("missing stub files")
	}
	if len(tt.Files.Test) == 0 {
		t.Fatalf("missing test files")
	}
}

func validateEmbeddedTaskFiles(t *testing.T, loader *Loader, tt *Task) {
	files := make([]string, 0, len(tt.Files.Stub)+len(tt.Files.Test)+len(tt.Files.HiddenTest)+len(tt.Files.Support))
	files = append(files, tt.Files.Stub...)
	files = append(files, tt.Files.Test...)
	files = append(files, tt.Files.HiddenTest...)
	files = append(files, tt.Files.Support...)

	for _, filename := range files {
		content, err := loader.ReadTaskFile(tt, filename)
		if err != nil {
			t.Fatalf("ReadTaskFile(%s) error: %v", filename, err)
		}
		if len(content) == 0 {
			t.Fatalf("ReadTaskFile(%s) returned empty content", filename)
		}
	}
}
