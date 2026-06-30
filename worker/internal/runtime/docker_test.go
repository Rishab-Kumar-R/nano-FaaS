package runtime

import (
	"strings"
	"testing"
)

func TestRuntimeConfig_Python(t *testing.T) {
	img, cmd, env := runtimeConfig("PYTHON", `print("hi")`)
	if img != "python:3.9-alpine" {
		t.Errorf("img = %q", img)
	}
	if cmd[0] != "python" || cmd[1] != "-c" {
		t.Errorf("cmd = %v", cmd)
	}
	if len(env) != 0 {
		t.Errorf("expected no env, got %v", env)
	}
}

func TestRuntimeConfig_Node(t *testing.T) {
	img, cmd, _ := runtimeConfig("NODEJS", `console.log("hi")`)
	if img != "node:18-alpine" {
		t.Errorf("img = %q", img)
	}
	if cmd[0] != "node" || cmd[1] != "-e" {
		t.Errorf("cmd = %v", cmd)
	}
}

func TestRuntimeConfig_Go(t *testing.T) {
	img, cmd, env := runtimeConfig("GO", `fmt.Println("hi")`)
	if img != "golang:1.21-alpine" {
		t.Errorf("img = %q", img)
	}
	if len(cmd) == 0 || cmd[0] != "sh" {
		t.Errorf("cmd = %v", cmd)
	}
	if len(env) == 0 || !strings.HasPrefix(env[0], "NANO_CODE=") {
		t.Errorf("env = %v", env)
	}
}

func TestWrapGoCode_NoPackage(t *testing.T) {
	code := `fmt.Println("hello")`
	wrapped := wrapGoCode(code)
	if !strings.Contains(wrapped, "package main") {
		t.Error("expected package main")
	}
	if !strings.Contains(wrapped, "func main()") {
		t.Error("expected func main()")
	}
	if !strings.Contains(wrapped, code) {
		t.Error("original code missing from wrapped output")
	}
}

func TestWrapGoCode_AlreadyHasPackage(t *testing.T) {
	code := "package main\n\nfunc main() {\n}\n"
	wrapped := wrapGoCode(code)
	if wrapped != code {
		t.Error("code with package main should be returned unchanged")
	}
}

func TestWrapGoCode_AppliedInRuntimeConfig(t *testing.T) {
	snippet := `fmt.Println("wrapped")`
	_, _, env := runtimeConfig("GO", snippet)
	nanoCode := strings.TrimPrefix(env[0], "NANO_CODE=")
	if !strings.Contains(nanoCode, "package main") {
		t.Error("runtimeConfig should auto-wrap Go snippets")
	}
}
