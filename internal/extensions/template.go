package extensions

import (
	"path/filepath"
	"strings"
)

type TemplateContext struct {
	Path         string
	MetaPath     string
	Name         string
	RecordingDir string
	ConfigDir    string
}

func ExpandArgs(args []string, ctx TemplateContext) []string {
	values := map[string]string{
		"{{path}}":          ctx.Path,
		"{{meta_path}}":     ctx.MetaPath,
		"{{name}}":          ctx.Name,
		"{{recording_dir}}": ctx.RecordingDir,
		"{{config_dir}}":    ctx.ConfigDir,
	}
	expanded := make([]string, len(args))
	for i, arg := range args {
		for token, value := range values {
			arg = strings.ReplaceAll(arg, token, value)
		}
		expanded[i] = arg
	}
	return expanded
}

func TemplateContextFor(path, metaPath, configDir string) TemplateContext {
	return TemplateContext{
		Path:         path,
		MetaPath:     metaPath,
		Name:         filepath.Base(path),
		RecordingDir: filepath.Dir(path),
		ConfigDir:    configDir,
	}
}
