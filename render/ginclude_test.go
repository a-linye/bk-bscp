/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package render

import (
	"fmt"
	"testing"
)

func TestExpandGinclude_NoGinclude(t *testing.T) {
	content := "hello world\nno ginclude here"
	result, err := ExpandGinclude(content, func(name string) (string, error) {
		return "", fmt.Errorf("should not be called")
	}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != content {
		t.Fatalf("expected %q, got %q", content, result)
	}
}

func TestExpandGinclude_SingleLevel(t *testing.T) {
	templates := map[string]string{
		"header": "HEADER_CONTENT",
	}
	content := "before\n# Ginclude \"header\"\nafter"
	result, err := ExpandGinclude(content, func(name string) (string, error) {
		if c, ok := templates[name]; ok {
			return c, nil
		}
		return "", fmt.Errorf("template %q not found", name)
	}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "before\nHEADER_CONTENT\nafter"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestExpandGinclude_SingleQuotes(t *testing.T) {
	templates := map[string]string{
		"header": "HEADER_CONTENT",
	}
	content := "# Ginclude 'header'"
	result, err := ExpandGinclude(content, func(name string) (string, error) {
		if c, ok := templates[name]; ok {
			return c, nil
		}
		return "", fmt.Errorf("template %q not found", name)
	}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "HEADER_CONTENT" {
		t.Fatalf("expected %q, got %q", "HEADER_CONTENT", result)
	}
}

func TestExpandGinclude_MultiLevel(t *testing.T) {
	templates := map[string]string{
		"A": "content_A with\n# Ginclude \"B\"",
		"B": "content_B with\n# Ginclude \"C\"",
		"C": "content_C",
	}
	content := "# Ginclude \"A\""
	result, err := ExpandGinclude(content, func(name string) (string, error) {
		if c, ok := templates[name]; ok {
			return c, nil
		}
		return "", fmt.Errorf("template %q not found", name)
	}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "content_A with\ncontent_B with\ncontent_C"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestExpandGinclude_CircularReference(t *testing.T) {
	templates := map[string]string{
		"A": "# Ginclude \"B\"",
		"B": "# Ginclude \"A\"",
	}
	_, err := ExpandGinclude(templates["A"], func(name string) (string, error) {
		if c, ok := templates[name]; ok {
			return c, nil
		}
		return "", fmt.Errorf("template %q not found", name)
	}, 10)
	if err == nil {
		t.Fatal("expected circular reference error, got nil")
	}
}

func TestExpandGinclude_MaxDepthExceeded(t *testing.T) {
	// Create a chain that exceeds depth
	templates := map[string]string{}
	for i := 0; i < 15; i++ {
		next := fmt.Sprintf("tmpl_%d", i+1)
		templates[fmt.Sprintf("tmpl_%d", i)] = fmt.Sprintf("# Ginclude \"%s\"", next)
	}
	templates["tmpl_15"] = "end"

	_, err := ExpandGinclude(templates["tmpl_0"], func(name string) (string, error) {
		if c, ok := templates[name]; ok {
			return c, nil
		}
		return "", fmt.Errorf("template %q not found", name)
	}, 10)
	if err == nil {
		t.Fatal("expected max depth error, got nil")
	}
}

func TestExpandGinclude_TemplateNotFound(t *testing.T) {
	content := "# Ginclude \"nonexistent\""
	_, err := ExpandGinclude(content, func(name string) (string, error) {
		return "", fmt.Errorf("template %q not found", name)
	}, 10)
	if err == nil {
		t.Fatal("expected error for nonexistent template, got nil")
	}
}

func TestExpandGinclude_MultipleIncludes(t *testing.T) {
	templates := map[string]string{
		"header": "HEADER",
		"footer": "FOOTER",
	}
	content := "# Ginclude \"header\"\nmiddle\n# Ginclude \"footer\""
	result, err := ExpandGinclude(content, func(name string) (string, error) {
		if c, ok := templates[name]; ok {
			return c, nil
		}
		return "", fmt.Errorf("template %q not found", name)
	}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "HEADER\nmiddle\nFOOTER"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestExpandGinclude_ExtraSpaces(t *testing.T) {
	templates := map[string]string{
		"tmpl": "CONTENT",
	}
	content := "#   Ginclude   \"tmpl\"  "
	result, err := ExpandGinclude(content, func(name string) (string, error) {
		if c, ok := templates[name]; ok {
			return c, nil
		}
		return "", fmt.Errorf("template %q not found", name)
	}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "CONTENT" {
		t.Fatalf("expected %q, got %q", "CONTENT", result)
	}
}

func TestExpandGinclude_NotAtLineStart(t *testing.T) {
	// Ginclude not at line start should not be matched
	content := "  # Ginclude \"tmpl\""
	result, err := ExpandGinclude(content, func(name string) (string, error) {
		return "", fmt.Errorf("should not be called")
	}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != content {
		t.Fatalf("expected content unchanged, got %q", result)
	}
}
