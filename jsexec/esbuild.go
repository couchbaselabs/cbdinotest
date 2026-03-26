package jsexec

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/evanw/esbuild/pkg/api"
)

// dirnamePlugin returns an esbuild plugin that injects per-file
// __dirname and __filename constants at the top of every .js/.ts
// file during loading. Each file gets its own actual directory
// and filename, not the entry point's.
func dirnamePlugin() api.Plugin {
	return api.Plugin{
		Name: "dirname",
		Setup: func(build api.PluginBuild) {
			build.OnLoad(api.OnLoadOptions{Filter: `\.(js|ts)$`},
				func(args api.OnLoadArgs) (api.OnLoadResult, error) {
					contents, err := os.ReadFile(args.Path)
					if err != nil {
						return api.OnLoadResult{}, err
					}

					dir := filepath.Dir(args.Path)

					injected := fmt.Sprintf(
						"const __dirname = %q;\nconst __filename = %q;\n",
						dir, args.Path,
					) + string(contents)

					loader := api.LoaderTS
					if filepath.Ext(args.Path) == ".js" {
						loader = api.LoaderJS
					}

					return api.OnLoadResult{
						Contents: &injected,
						Loader:   loader,
					}, nil
				})
		},
	}
}

// cbdtPlugin returns an esbuild plugin that resolves "cbdt:*" imports
// using the provided module shim registry. Each entry maps a module
// path (e.g. "cbdt:http") to a JS shim string.
func cbdtPlugin(moduleShims map[string]string) api.Plugin {
	return api.Plugin{
		Name: "cbdt",
		Setup: func(build api.PluginBuild) {
			build.OnResolve(api.OnResolveOptions{Filter: `^cbdt:`},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					return api.OnResolveResult{
						Path:      args.Path,
						Namespace: "cbdt",
					}, nil
				})

			build.OnLoad(api.OnLoadOptions{Filter: `.*`, Namespace: "cbdt"},
				func(args api.OnLoadArgs) (api.OnLoadResult, error) {
					shim, ok := moduleShims[args.Path]
					if !ok {
						return api.OnLoadResult{}, fmt.Errorf("unknown cbdt module: %s", args.Path)
					}
					return api.OnLoadResult{
						Contents: &shim,
						Loader:   api.LoaderJS,
					}, nil
				})
		},
	}
}

// Bundle bundles a JS/TS file with esbuild, resolving cbdt:* modules
// using the provided shim registry. The globalName controls the IIFE
// export name (e.g. "__workload").
func Bundle(filePath string, globalName string, moduleShims map[string]string) (string, error) {
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{filePath},
		Bundle:      true,
		Write:       false,
		Format:      api.FormatIIFE,
		GlobalName:  globalName,
		Plugins:     []api.Plugin{cbdtPlugin(moduleShims), dirnamePlugin()},
	})

	if len(result.Errors) > 0 {
		for _, msg := range result.Errors {
			if msg.Location != nil {
				log.Printf("esbuild error: %s:%d:%d: %s",
					msg.Location.File, msg.Location.Line, msg.Location.Column, msg.Text)
			} else {
				log.Printf("esbuild error: %s", msg.Text)
			}
			for _, note := range msg.Notes {
				if note.Location != nil {
					log.Printf("  note: %s:%d:%d: %s",
						note.Location.File, note.Location.Line, note.Location.Column, note.Text)
				} else {
					log.Printf("  note: %s", note.Text)
				}
			}
		}
		return "", fmt.Errorf("esbuild bundling failed with %d error(s)", len(result.Errors))
	}

	if len(result.OutputFiles) == 0 {
		return "", fmt.Errorf("esbuild produced no output")
	}

	return string(result.OutputFiles[0].Contents), nil
}
