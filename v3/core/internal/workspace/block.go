package workspace

import (
	"errors"
	"strings"
)

// Managed block markers. A user-owned file may contain at most one
// such block; everything outside the block belongs to the user.
const (
	BlockStart = "# >>> veilbox managed >>>"
	BlockEnd   = "# <<< veilbox managed <<<"
)

// ErrMultipleBlocks is returned when a user file contains more than one
// Veilbox managed block (ambiguous state; never repaired).
var ErrMultipleBlocks = errors.New("multiple veilbox managed blocks")

// Block is a located managed block inside a user-owned file.
type Block struct {
	// Start is the byte offset of the BlockStart line.
	Start int
	// End is the byte offset just past the BlockEnd line.
	End int
	// Content is the text between the markers, including their lines.
	Content string
}

// FindBlock scans content for Veilbox managed blocks.
//
//	0 blocks: block == nil, ok == true
//	1 block:  block != nil, ok == true
//	>1 block: ErrMultipleBlocks
func FindBlock(content string) (*Block, error) {
	start := strings.Index(content, BlockStart)
	if start < 0 {
		return nil, nil
	}
	endRel := strings.Index(content[start:], BlockEnd)
	if endRel < 0 {
		return nil, errors.New("veilbox managed block starts but never ends")
	}
	end := start + endRel + len(BlockEnd)
	if strings.HasSuffix(content[:end], "\r") {
		end++ // keep CRLF endings consistent
	}
	if end < len(content) && content[end] == '\n' {
		end++
	} else if end < len(content) && content[end] != '\n' {
		end = start + endRel // marker mid-line: do not extend beyond it
	}
	blk := &Block{Start: start, End: end, Content: content[start:end]}
	if strings.Contains(content[end:], BlockStart) {
		return nil, ErrMultipleBlocks
	}
	return blk, nil
}

// Interior returns the text between the markers, trimmed of the marker
// lines. This is the Veilbox-managed payload.
func (b *Block) Interior() string {
	lines := strings.Split(b.Content, "\n")
	var out []string
	seenStart := false
	for _, l := range lines {
		if !seenStart {
			if strings.Contains(l, BlockStart) {
				seenStart = true
			}
			continue
		}
		if strings.Contains(l, BlockEnd) {
			break
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// InsertBlock appends a managed block to content, preserving every
// existing byte. The file is left with a single trailing newline after
// the block.
func InsertBlock(content, blockText string) string {
	out := content
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out + blockText
}

// ReplaceBlock swaps the interior of an existing block for blockText,
// preserving everything outside the block. If the block text is
// already present verbatim, content is returned unchanged.
func ReplaceBlock(content string, blk *Block, blockText string) string {
	if blk.Content == blockText {
		return content
	}
	return content[:blk.Start] + blockText + content[blk.End:]
}

// RemoveBlock strips a managed block from content, including its
// trailing newline. Everything else — including a preceding newline
// that may belong to user content — is preserved.
func RemoveBlock(content string, blk *Block) string {
	if blk.End <= len(content) && blk.End > blk.Start {
		return content[:blk.Start] + content[blk.End:]
	}
	return content
}

// BlockText builds a block payload from a payload line list.
func BlockText(stateDir string, payload []string) string {
	var b strings.Builder
	b.WriteString(BlockStart + "\n")
	for _, line := range payload {
		b.WriteString(line + "\n")
	}
	b.WriteString(BlockEnd + "\n")
	return b.String()
}
