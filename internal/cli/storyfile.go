package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jason-i-magno/career/internal/model"
)

// Stories are edited as a small markdown document in $EDITOR rather than
// through a stack of flags: STAR answers are paragraphs, and paragraphs belong
// in a text editor.

const storyTemplateHelp = `<!--
Fill in each section. Guidance:

  Situation  One or two sentences of context. What was at stake?
  Task       Your specific responsibility. "I was asked to…", not "we".
  Action     What YOU did, and why you chose it over the alternative.
             This is the longest section and the one interviewers grade.
  Result     The outcome, quantified. If you cannot quantify it, say what
             changed for the team or the user.
  Metrics    The number you will say out loud. Keep it relative
             ("3x throughput", "cut deploy time from 6h to 40m") rather
             than absolute where the absolute figure is sensitive.

  Competencies  Comma-separated, from:
                ownership, scale, debugging, conflict, failure,
                mentoring, ambiguity, influence, delivery, collab

  Sanitized     yes / no. Say yes only when this telling contains no
                program names, no customer identity, no capability
                specifics, and no figures that are sensitive in
                aggregate. Only sanitized stories count as ready, and
                only sanitized stories should be rehearsed out loud —
                the version you practise is the version that surfaces
                under interview pressure.

Lines in this comment block are ignored.
-->
`

func renderStoryFile(s model.Story) string {
	comps := make([]string, len(s.Competencies))
	for i, c := range s.Competencies {
		comps[i] = string(c)
	}
	sanitized := "no"
	if s.Sanitized {
		sanitized = "yes"
	}
	return fmt.Sprintf(`# %s

## Situation
%s

## Task
%s

## Action
%s

## Result
%s

## Metrics
%s

## Competencies
%s

## Sanitized
%s

%s`, s.Title, s.Situation, s.Task, s.Action, s.Result, s.Metrics,
		strings.Join(comps, ", "), sanitized, storyTemplateHelp)
}

// parseStoryFile reads the edited document back into a Story, preserving the
// caller's ID and timestamps.
func parseStoryFile(src string, into model.Story) (model.Story, error) {
	src = stripComments(src)

	sections := map[string]*strings.Builder{}
	var current *strings.Builder
	title := into.Title

	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "## "):
			key := strings.ToLower(strings.TrimSpace(trimmed[3:]))
			b := &strings.Builder{}
			sections[key] = b
			current = b
			continue
		case strings.HasPrefix(trimmed, "# "):
			title = strings.TrimSpace(trimmed[2:])
			current = nil
			continue
		}
		if current != nil {
			current.WriteString(line)
			current.WriteString("\n")
		}
	}

	get := func(key string) string {
		b, ok := sections[key]
		if !ok {
			return ""
		}
		return strings.TrimSpace(b.String())
	}

	out := into
	out.Title = strings.TrimSpace(title)
	if out.Title == "" {
		return model.Story{}, fmt.Errorf("story needs a title on the `# ` line")
	}
	out.Situation = get("situation")
	out.Task = get("task")
	out.Action = get("action")
	out.Result = get("result")
	out.Metrics = get("metrics")

	out.Competencies = nil
	for _, raw := range strings.FieldsFunc(get("competencies"), func(r rune) bool {
		return r == ',' || r == '\n'
	}) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		c, err := model.ParseCompetency(raw)
		if err != nil {
			return model.Story{}, err
		}
		out.Competencies = append(out.Competencies, c)
	}
	out.Competencies = model.SortCompetencies(out.Competencies)

	switch strings.ToLower(get("sanitized")) {
	case "yes", "y", "true":
		out.Sanitized = true
	case "no", "n", "false", "":
		out.Sanitized = false
	default:
		return model.Story{}, fmt.Errorf("Sanitized must be yes or no, got %q", get("sanitized"))
	}
	return out, nil
}

// editInEditor writes content to a temp file, opens $EDITOR on it, and returns
// the edited text. It reports an error if the user left the file unchanged.
func editInEditor(content, suffix string) (string, error) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}

	dir, err := os.MkdirTemp("", "career-story-")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "story"+suffix)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("writing draft: %w", err)
	}

	// The editor inherits the terminal so that full-screen editors work.
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("running editor %q: %w", editor, err)
	}

	edited, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading edited draft: %w", err)
	}
	return string(edited), nil
}

// stripComments removes HTML comment blocks, which is how every editable
// document here carries inline guidance without it becoming content.
func stripComments(src string) string {
	for {
		start := strings.Index(src, "<!--")
		if start < 0 {
			return src
		}
		end := strings.Index(src[start:], "-->")
		if end < 0 {
			return src[:start]
		}
		src = src[:start] + src[start+end+3:]
	}
}
