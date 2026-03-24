package main

import (
	"bufio"
	"bytes"
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed templates/*
var templateFS embed.FS

type Skill struct {
	Dir         string
	Name        string
	Description string
	Content     string // raw markdown body (after frontmatter)
	Excerpt     string // first non-empty line of content
}

// parseFrontmatter splits a SKILL.md into front matter fields and body.
func parseFrontmatter(raw string) (fields map[string]string, body string) {
	fields = map[string]string{}
	lines := strings.Split(raw, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return fields, raw
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return fields, raw
	}

	var curKey string
	var buf bytes.Buffer
	for _, line := range lines[1:end] {
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			if curKey != "" {
				fields[curKey] = strings.TrimSpace(buf.String())
				buf.Reset()
			}
			kv := strings.SplitN(line, ":", 2)
			curKey = strings.TrimSpace(kv[0])
			if len(kv) == 2 {
				val := strings.TrimSpace(kv[1])
				if val != ">" && val != "|" {
					buf.WriteString(val)
				}
			}
		} else {
			if buf.Len() > 0 {
				buf.WriteString(" ")
			}
			buf.WriteString(strings.TrimSpace(line))
		}
	}
	if curKey != "" {
		fields[curKey] = strings.TrimSpace(buf.String())
	}

	body = strings.Join(lines[end+1:], "\n")
	return fields, body
}

func loadSkills(root string) ([]Skill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillFile := filepath.Join(root, e.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue // not a skill dir
		}
		fields, body := parseFrontmatter(string(data))
		name := fields["name"]
		if name == "" {
			name = e.Name()
		}
		desc := fields["description"]

		// extract excerpt: first non-blank line of body after the h1
		excerpt := ""
		scanner := bufio.NewScanner(strings.NewReader(body))
		inExcerpt := false
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "#") {
				inExcerpt = true
				continue
			}
			if inExcerpt && line != "" {
				excerpt = line
				break
			}
		}

		skills = append(skills, Skill{
			Dir:         e.Name(),
			Name:        name,
			Description: desc,
			Content:     body,
			Excerpt:     excerpt,
		})
	}
	return skills, nil
}

func main() {
	// Skills live one level up from the marketplace dir
	repoRoot := filepath.Join(filepath.Dir(must(os.Executable())), "..")
	// When running via `go run`, executable is in a temp dir; use relative path instead.
	if _, err := os.Stat(filepath.Join(repoRoot, "go", "SKILL.md")); err != nil {
		repoRoot = ".."
	}

	tmpl, err := template.New("").Funcs(template.FuncMap{
		"slugify": func(s string) string { return strings.ReplaceAll(s, " ", "-") },
	}).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		skills, err := loadSkills(repoRoot)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		q := strings.ToLower(r.URL.Query().Get("q"))
		if q != "" {
			var filtered []Skill
			for _, s := range skills {
				if strings.Contains(strings.ToLower(s.Name), q) ||
					strings.Contains(strings.ToLower(s.Description), q) {
					filtered = append(filtered, s)
				}
			}
			skills = filtered
		}
		data := map[string]any{
			"Skills": skills,
			"Query":  r.URL.Query().Get("q"),
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
			log.Println(err)
		}
	})

	http.HandleFunc("/skill/", func(w http.ResponseWriter, r *http.Request) {
		dir := strings.TrimPrefix(r.URL.Path, "/skill/")
		dir = strings.Trim(dir, "/")
		skills, err := loadSkills(repoRoot)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		var skill *Skill
		for i := range skills {
			if skills[i].Dir == dir {
				skill = &skills[i]
				break
			}
		}
		if skill == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "skill.html", skill); err != nil {
			log.Println(err)
		}
	})

	addr := ":8080"
	log.Printf("Claude Code Skill Marketplace running at http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func must(s string, err error) string {
	if err != nil {
		return "."
	}
	return s
}
