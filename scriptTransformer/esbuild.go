package scriptTransformer

import (
	"fmt"
	"github.com/evanw/esbuild/pkg/api"
	"os"
	"path"
	"slices"
	"strings"
)

var gAllowsScriptExtensions = []string{".js", ".ts", ".tsx"}

type ScriptCompilationError struct {
	message string
}

func (m *ScriptCompilationError) Error() string {
	return m.message
}

func CompileJavascriptFile(scriptPath string, scriptPrefix string) (string, string, error) {
	if scriptPrefix == "" {
		scriptPrefix = "import '@progp/core'"
	}
	fileExt := path.Ext(scriptPath)

	if slices.Contains(gAllowsScriptExtensions, fileExt) {
		compileResult, err := bundleJavascriptScriptEntryPoint(scriptPath, scriptPrefix, true)
		if err != nil {
			return "", "", err
		}

		return compileResult.CompiledScriptContent, compileResult.CompiledScriptPath, nil
	}

	_, _ = fmt.Fprintf(os.Stdout, "unsupported script type: %s", fileExt)
	return "", "", nil
}

func bundleJavascriptScriptEntryPoint(scriptSourcePath string, scriptPrefix string, forceRebuild bool) (*TransformedScript, error) {
	outputDir := path.Join(GetCompileCacheDir(path.Dir(scriptSourcePath)), calcMd5(scriptSourcePath))

	compiledScriptBasePath := path.Join(outputDir, "stdin")

	if !forceRebuild && FileExists(compiledScriptBasePath+".js") {
		jsPath := compiledScriptBasePath + ".js"
		mapPath := jsPath + ".map"

		asBytes, err := os.ReadFile(jsPath)
		if err != nil {
			return bundleJavascriptScriptEntryPoint(scriptSourcePath, scriptPrefix, true)
		}
		//
		jsContent := string(asBytes)

		asBytes, err = os.ReadFile(mapPath)
		if err != nil {
			return bundleJavascriptScriptEntryPoint(scriptSourcePath, scriptPrefix, true)
		}
		//
		mapContent := string(asBytes)

		return &TransformedScript{
			OutputDir:             outputDir,
			CompiledScriptPath:    jsPath,
			CompiledScriptContent: jsContent,
			SourceMapScriptPath:   mapPath,
			SourceMapFileContent:  mapContent,
		}, nil
	}

	baseDir, entryPoint := path.Split(scriptSourcePath)

	buildOptions := api.BuildOptions{
		// Allow to replace react officiel package by progp implementation.
		// Note: commented du a matter in esbuild doing that is never requested if not found.
		/*Alias: map[string]string{
			"react": "@progp/react",
		},*/

		// Will allow forcing loading of the progp lib when starring.
		// Is required because this lib declares special common stuff like console and setTimeout.
		//
		Stdin: &api.StdinOptions{
			Contents: scriptPrefix + "\nimport './" + entryPoint + "';",

			// These are all optional:
			ResolveDir: baseDir,
			Loader:     api.LoaderTSX,
		},

		// Allows JSX syntax support.
		// Https://esbuild.github.io/api/#jsx
		//
		JSX: api.JSXTransform,

		//EntryPoints:   []string{entryPoint},
		AbsWorkingDir: baseDir,

		// Say where to search packages.
		// Allows having our own package directory in order to override current packages.
		// Https://esbuild.github.io/api/#node-paths
		//NodePaths: []string{"node_modules"},

		// https://esbuild.github.io/api/#platform
		//Platform: api.PlatformBrowser,

		// https://esbuild.github.io/api/#target
		//
		Engines: []api.Engine{
			//{exposedName: api.EngineChrome, Version: "100"},
			{Name: api.EngineNode, Version: "18"},
		},

		// Avoid enclosing into a function.
		// The matter is that enclosing hide async errors.
		// https://esbuild.github.io/api/#format
		//
		Format: api.FormatESModule,

		// Required for sourcemap.
		Write: true,

		// Make one uniq file will all dependencies.
		Bundle: true,

		// Where to put the outputs.
		// Only available if mustWrite is true.
		Outdir: outputDir,

		// Will copy the working file into the target dir (Outdir).
		Outbase: baseDir,

		//DropLabels: dropLabels,
		//Define: allDefines,

		// For code security analysis.
		// Will provide a report inside result.Metafile
		//
		Metafile: false,

		Plugins: getPlugins(),

		Sourcemap:      api.SourceMapLinked,
		SourcesContent: api.SourcesContentInclude,
		SourceRoot:     outputDir,
	}

	result := api.Build(buildOptions)

	if len(result.Errors) > 0 {
		errMsg := ""

		for _, err := range result.Errors {
			messages := []api.Message{
				{
					Text: err.Text,
				},
			}

			if err.Location != nil {
				messages[0].Location = &api.Location{
					File:     err.Location.File,
					Line:     err.Location.Line,
					Column:   err.Location.Column,
					Length:   err.Location.Length,
					LineText: err.Location.LineText,
				}
			}

			formatted := api.FormatMessages(messages, api.FormatMessagesOptions{
				Kind:          api.ErrorMessage,
				Color:         true,
				TerminalWidth: 160,
			})

			errMsg += strings.Join(formatted, "\n")
			//fmt.Printf("%s", strings.Join(formatted, "\n"))
		}

		return nil, &ScriptCompilationError{message: errMsg}
	}

	callResult := TransformedScript{}
	callResult.OutputDir = outputDir

	_ = os.WriteFile(path.Join(outputDir, "meta.json"), []byte(result.Metafile), os.ModePerm)

	for _, output := range result.OutputFiles {
		if strings.HasSuffix(output.Path, ".js") {
			callResult.CompiledScriptPath = output.Path
			callResult.CompiledScriptContent = string(output.Contents)
		} else if strings.HasSuffix(output.Path, ".map") {
			callResult.SourceMapScriptPath = output.Path
			callResult.SourceMapFileContent = string(output.Contents)
		}
	}

	return &callResult, nil
}
