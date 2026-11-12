package skills

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"viximage/internal/client"
)

//go:embed edit_image.py
var editScript string

// Context carries shared resources for both skills.
type Context struct {
	Client  *client.Client
	OutDir  string // where produced images are saved
	WorkDir string // where python/temp files live
	Python  string
	// Engine selects the image-editing backend: "go" (native, default) or
	// "python" (legacy PIL script, kept as a fallback).
	Engine string
	Log    func(format string, args ...any)
}

func (c *Context) logf(f string, a ...any) {
	if c.Log != nil {
		c.Log(f, a...)
	}
}

// Edit renders a composition spec to an image.
// The Go-native renderer is the default; the legacy Python/PIL path is kept
// for environments where the native font stack is unavailable.
func (c *Context) Edit(spec map[string]any) (string, error) {
	if err := os.MkdirAll(c.WorkDir, 0o755); err != nil {
		return "", err
	}
	out, _ := spec["output"].(string)
	if out == "" {
		out = filepath.Join(c.OutDir, fmt.Sprintf("edit_%d.png", os.Getpid()))
		spec["output"] = out
	}

	if c.Engine != "python" {
		b, err := json.Marshal(spec)
		if err != nil {
			return "", err
		}
		var s Spec
		if err := json.Unmarshal(b, &s); err != nil {
			return "", fmt.Errorf("解析规格失败: %w", err)
		}
		s.Output = out
		if _, err := RenderSpecToFile(&s); err == nil {
			c.logf("[编辑] Go 原生渲染完成: %s", out)
			return out, nil
		} else if c.Engine == "go" {
			// If native rendering fails hard, fall back to Python when available.
			if p, perr := c.editPython(spec); perr == nil {
				c.logf("[编辑] Go 渲染失败，已回退 Python: %s", p)
				return p, nil
			}
			return "", err
		}
	}
	return c.editPython(spec)
}

// editPython renders the spec with the embedded PIL script.
func (c *Context) editPython(spec map[string]any) (string, error) {
	scriptPath := filepath.Join(c.WorkDir, "edit_image.py")
	if err := os.WriteFile(scriptPath, []byte(editScript), 0o755); err != nil {
		return "", err
	}
	specPath := filepath.Join(c.WorkDir, "spec.json")
	b, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(specPath, b, 0o644); err != nil {
		return "", err
	}
	out, _ := spec["output"].(string)
	if out == "" {
		out = filepath.Join(c.OutDir, fmt.Sprintf("edit_%d.png", os.Getpid()))
		spec["output"] = out
		_ = os.WriteFile(specPath, b, 0o644)
	}
	cmd := exec.Command(c.Python, scriptPath, "--spec", specPath)
	var stderr []byte
	stderr, err = cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("编辑技能失败: %w (%s)", err, string(stderr))
	}
	c.logf("[编辑] Python 合成完成: %s", out)
	return out, nil
}
