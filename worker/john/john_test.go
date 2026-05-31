package john

import (
	"os"
	"testing"

	"go.uber.org/zap"
)

func TestArgsIncludeValuedAndValuelessFlags(t *testing.T) {
	cmd := New("john", "hashes.txt", map[string]string{
		"--format": "raw-sha256",
		"--single": "",
		"--node":   "1/5",
	}, zap.NewNop())

	args := cmd.args()
	want := map[string]bool{
		"hashes.txt":          true,
		potFlag:               true,
		"--format=raw-sha256": true,
		"--single":            true,
		"--node=1/5":          true,
	}

	if len(args) != len(want) {
		t.Fatalf("got %d args, want %d: %v", len(args), len(want), args)
	}
	for _, arg := range args {
		if !want[arg] {
			t.Fatalf("unexpected arg %q in %v", arg, args)
		}
	}
}

func TestReadPotfileReturnsLinesWithoutTrailingEmptyLine(t *testing.T) {
	t.Chdir(t.TempDir())

	tests := []struct {
		name  string
		input string
	}{
		{name: "with trailing newline", input: "hash1:password1\nhash2:password2\n"},
		{name: "without trailing newline", input: "hash1:password1\nhash2:password2"},
	}

	want := []string{"hash1:password1", "hash2:password2"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(potFile, []byte(tt.input), 0o600); err != nil {
				t.Fatal(err)
			}

			cmd := New("john", "hashes.txt", nil, zap.NewNop())
			got := cmd.ReadPotfile()
			if len(got) != len(want) {
				t.Fatalf("got %d lines, want %d: %v", len(got), len(want), got)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("line %d = %q, want %q", i, got[i], want[i])
				}
			}
		})
	}
}

func TestReadPotfileReturnsNilForEmptyFile(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.WriteFile(potFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := New("john", "hashes.txt", nil, zap.NewNop())
	if got := cmd.ReadPotfile(); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}
