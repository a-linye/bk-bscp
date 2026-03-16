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
	"regexp"
	"strings"
)

// gincludeRegex matches lines like: # Ginclude "template_name" or # Ginclude 'template_name'
var gincludeRegex = regexp.MustCompile(`(?m)^#\s*Ginclude\s+["']([^"']+)["']\s*$`)

// ExpandGincludeFunc is a callback function to resolve referenced template content.
// Parameter: templateName (config template name)
// Returns: template content, error
type ExpandGincludeFunc func(templateName string) (string, error)

// ExpandGinclude recursively expands Ginclude directives in template content.
// content: original template content
// resolver: callback to get template content by name
// maxDepth: maximum recursion depth to prevent circular references (recommended: 10)
func ExpandGinclude(content string, resolver ExpandGincludeFunc, maxDepth int) (string, error) {
	visited := make(map[string]bool)
	return expandGinclude(content, resolver, maxDepth, 0, visited)
}

func expandGinclude(content string, resolver ExpandGincludeFunc, maxDepth, currentDepth int, visited map[string]bool) (string, error) {
	if currentDepth > maxDepth {
		return "", fmt.Errorf("ginclude max depth %d exceeded, possible circular reference", maxDepth)
	}

	matches := gincludeRegex.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	var result strings.Builder
	lastIndex := 0

	for _, match := range matches {
		// match[0]:match[1] is the full match, match[2]:match[3] is the template name
		fullMatchStart := match[0]
		fullMatchEnd := match[1]
		templateName := content[match[2]:match[3]]

		// Check for direct circular reference
		if visited[templateName] {
			return "", fmt.Errorf("ginclude circular reference detected: template %q", templateName)
		}

		// Resolve the template content
		resolvedContent, err := resolver(templateName)
		if err != nil {
			return "", fmt.Errorf("ginclude resolve template %q failed: %w", templateName, err)
		}

		// Mark as visited for circular reference detection
		visited[templateName] = true

		// Recursively expand the resolved content
		expandedContent, err := expandGinclude(resolvedContent, resolver, maxDepth, currentDepth+1, visited)
		if err != nil {
			return "", err
		}

		// Unmark after recursive expansion (allow same template in different branches)
		delete(visited, templateName)

		result.WriteString(content[lastIndex:fullMatchStart])
		result.WriteString(expandedContent)
		lastIndex = fullMatchEnd
	}

	result.WriteString(content[lastIndex:])
	return result.String(), nil
}
