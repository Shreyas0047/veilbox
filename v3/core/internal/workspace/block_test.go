package workspace

import (
	"strings"
	"testing"
)

func blockText() string {
	return BlockText("/home/user/.config/veilbox/workspace", []string{
		`[ -f "/home/user/.config/veilbox/workspace/shell.sh" ] && . "/home/user/.config/veilbox/workspace/shell.sh"`,
	})
}

func TestFindBlockNone(t *testing.T) {
	blk, err := FindBlock("alias ll='ls -la'\n")
	if err != nil || blk != nil {
		t.Fatalf("expected no block, got %v err=%v", blk, err)
	}
}

func TestFindBlockSingle(t *testing.T) {
	content := "user stuff\n" + blockText() + "more user stuff\n"
	blk, err := FindBlock(content)
	if err != nil {
		t.Fatal(err)
	}
	if blk == nil || !strings.Contains(blk.Content, BlockStart) || !strings.Contains(blk.Content, BlockEnd) {
		t.Fatalf("bad block: %q", blk.Content)
	}
	before := content[:blk.Start]
	after := content[blk.End:]
	if before != "user stuff\n" || after != "more user stuff\n" {
		t.Fatalf("block boundaries wrong: before=%q after=%q", before, after)
	}
}

func TestFindBlockMultiple(t *testing.T) {
	content := blockText() + "\n" + blockText()
	if _, err := FindBlock(content); err != ErrMultipleBlocks {
		t.Fatalf("expected ErrMultipleBlocks, got %v", err)
	}
}

func TestFindBlockUnterminated(t *testing.T) {
	content := "# >>> veilbox managed >>>\nsomething"
	if _, err := FindBlock(content); err == nil {
		t.Fatal("expected error for unterminated block")
	}
}

func TestFindBlockNoTrailingNewline(t *testing.T) {
	content := "user line\n" + strings.TrimSuffix(blockText(), "\n")
	blk, err := FindBlock(content)
	if err != nil {
		t.Fatal(err)
	}
	if blk == nil || blk.End != len(content) {
		t.Fatalf("block must extend to EOF: %+v len=%d", blk, len(content))
	}
}

func TestInsertBlockPreservesUserContent(t *testing.T) {
	user := "# my dotfile\nalias ll='ls -la'\nexport FOO=bar\n"
	got := InsertBlock(user, blockText())
	if !strings.HasPrefix(got, user) {
		t.Fatalf("insert must append, got:\n%s", got)
	}
	if !strings.Contains(got, BlockStart) {
		t.Fatalf("missing block: %s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatal("file must end with newline")
	}
	// re-parse: exactly one block
	blk, err := FindBlock(got)
	if err != nil || blk == nil {
		t.Fatalf("inserted block must parse: %v %v", err, blk)
	}
}

func TestInsertBlockNoTrailingNewlineInput(t *testing.T) {
	got := InsertBlock("no newline", blockText())
	if !strings.HasSuffix(got, "\n") || !strings.Contains(got, "no newline\n# >>>") {
		t.Fatalf("newline normalization wrong:\n%q", got)
	}
}

func TestReplaceBlock(t *testing.T) {
	user := "keep me\n" + blockText() + "keep me too\n"
	blk, err := FindBlock(user)
	if err != nil {
		t.Fatal(err)
	}
	newBlock := BlockText("/other", []string{"line-2"})
	got := ReplaceBlock(user, blk, newBlock)
	if !strings.Contains(got, "keep me\n") || !strings.Contains(got, "keep me too\n") {
		t.Fatalf("user content lost:\n%s", got)
	}
	if !strings.Contains(got, "line-2") || strings.Contains(got, "shell.sh") {
		t.Fatalf("block not replaced:\n%s", got)
	}
	// identical content is a no-op
	if ReplaceBlock(user, blk, blk.Content) != user {
		t.Fatal("replace with identical content must be a no-op")
	}
}

func TestRemoveBlock(t *testing.T) {
	user := "keep me\n"
	content := user + blockText() + "tail\n"
	blk, err := FindBlock(content)
	if err != nil {
		t.Fatal(err)
	}
	got := RemoveBlock(content, blk)
	if strings.Contains(got, BlockStart) {
		t.Fatalf("block not removed:\n%s", got)
	}
	if !strings.Contains(got, "keep me\n") || !strings.Contains(got, "tail\n") {
		t.Fatalf("user content lost:\n%s", got)
	}
	if got != "keep me\n"+"tail\n" {
		t.Fatalf("unexpected residue: %q", got)
	}
}

func TestBlockInterior(t *testing.T) {
	blk, err := FindBlock(blockText())
	if err != nil {
		t.Fatal(err)
	}
	inner := blk.Interior()
	if strings.Contains(inner, BlockStart) || strings.Contains(inner, BlockEnd) {
		t.Fatalf("interior must exclude markers: %q", inner)
	}
	if !strings.Contains(inner, "shell.sh") {
		t.Fatalf("interior payload missing: %q", inner)
	}
}

func TestBlockTextDeterministic(t *testing.T) {
	a := blockText()
	b := blockText()
	if a != b {
		t.Fatal("block text must be deterministic")
	}
}
