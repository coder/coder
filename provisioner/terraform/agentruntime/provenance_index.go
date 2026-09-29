package agentruntime

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
)

const (
	maxRuntimeDirectoryEntries  = 100_000
	maxRuntimeSourceBytes       = 32 << 20
	maxRuntimeSourceFiles       = 100_000
	maxRuntimeSourceExpressions = 100_000
	maxRuntimeProvenanceDepth   = 256
)

var (
	errRuntimeDirectoryEntryLimit = xerrors.New(
		"agent runtime directory entry limit exceeded",
	)
	errRuntimeSourceFileLimit = xerrors.New(
		"agent runtime source file limit exceeded",
	)
)

type runtimeExpressionKind int

const (
	_ runtimeExpressionKind = iota
	runtimeExpressionLocal
	runtimeExpressionResource
	runtimeExpressionModuleCall
	runtimeExpressionOutput
)

type runtimeExpressionKey struct {
	moduleAddress string
	kind          runtimeExpressionKind
	resourceType  string
	name          string
	attribute     string
}

type provenanceLimits struct {
	directoryEntries  int
	manifestBytes     int
	sourceBytes       int
	sourceFiles       int
	sourceExpressions int
	referenceCount    int
	referenceBytes    int
	referenceDepth    int
	expressionDepth   int
}

func defaultProvenanceLimits() provenanceLimits {
	return provenanceLimits{
		directoryEntries:  maxRuntimeDirectoryEntries,
		manifestBytes:     maxRuntimeSourceBytes,
		sourceBytes:       maxRuntimeSourceBytes,
		sourceFiles:       maxRuntimeSourceFiles,
		sourceExpressions: maxRuntimeSourceExpressions,
		referenceCount:    maxRuntimeReferenceCount,
		referenceBytes:    maxRuntimeReferenceBytes,
		referenceDepth:    maxRuntimeProvenanceDepth,
		expressionDepth:   maxRuntimeProvenanceDepth,
	}
}

type provenanceBudget struct {
	limits provenanceLimits

	sourceExpressions int
	referenceCount    int
	referenceBytes    int
}

func newProvenanceBudget(limits provenanceLimits) *provenanceBudget {
	return &provenanceBudget{limits: limits}
}

func (b *provenanceBudget) consumeSourceExpression() error {
	if exceedsLimit(b.sourceExpressions, 1, b.limits.sourceExpressions) {
		return xerrors.Errorf(
			"agent runtime provenance exceeds the limit of %d expanded Terraform source expressions",
			b.limits.sourceExpressions,
		)
	}
	b.sourceExpressions++
	return nil
}

func (b *provenanceBudget) consumeReference(referenceBytes int) error {
	if exceedsLimit(b.referenceCount, 1, b.limits.referenceCount) {
		return xerrors.Errorf(
			"agent runtime provenance exceeds the limit of %d Terraform references",
			b.limits.referenceCount,
		)
	}
	if exceedsLimit(
		b.referenceBytes, referenceBytes, b.limits.referenceBytes,
	) {
		return xerrors.Errorf(
			"agent runtime provenance exceeds the limit of %d Terraform reference bytes",
			b.limits.referenceBytes,
		)
	}
	b.referenceCount++
	b.referenceBytes += referenceBytes
	return nil
}

func (b *provenanceBudget) checkDepth(depth int) error {
	if depth < b.limits.expressionDepth {
		return nil
	}
	return xerrors.Errorf(
		"agent runtime provenance exceeds the limit of %d nested Terraform expressions",
		b.limits.expressionDepth,
	)
}

func (b *provenanceBudget) checkReferenceDepth(depth int) error {
	if depth < b.limits.referenceDepth {
		return nil
	}
	return xerrors.Errorf(
		"agent runtime provenance exceeds the limit of %d indirect Terraform references",
		b.limits.referenceDepth,
	)
}

type runtimeModulesManifest struct {
	Modules []*runtimeModuleManifestEntry `json:"Modules"`
}

type runtimeModuleManifestEntry struct {
	Source  string `json:"Source"`
	Version string `json:"Version"`
	Key     string `json:"Key"`
	Dir     string `json:"Dir"`
}

var runtimeSourceSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "locals"},
		{Type: "resource", LabelNames: []string{"type", "name"}},
		{Type: "module", LabelNames: []string{"name"}},
		{Type: "output", LabelNames: []string{"name"}},
	},
}

func (i *configIndex) indexRuntimeSourceExpressions(
	ctx context.Context,
	workdir string,
) error {
	return i.indexRuntimeSourceExpressionsWithLimits(
		ctx, workdir, defaultProvenanceLimits(),
	)
}

func (i *configIndex) indexRuntimeSourceExpressionsWithLimits(
	ctx context.Context,
	workdir string,
	limits provenanceLimits,
) error {
	if i == nil {
		return xerrors.New("Terraform plan configuration is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	i.runtimeSourceIndexed = false
	clear(i.runtimeSourceExpressions)

	moduleDirs, err := i.runtimeModuleDirectoriesWithLimit(
		ctx, workdir, limits.manifestBytes,
	)
	if err != nil {
		return err
	}
	moduleAddressesByDirectory := map[string][]string{}
	resolvedDirectories := map[string]string{}
	var directories []string
	for _, moduleAddress := range slices.Sorted(maps.Keys(i.modules)) {
		if err := ctx.Err(); err != nil {
			return err
		}
		dir := moduleDirs[moduleAddress]
		if dir == "" {
			continue
		}
		absoluteDir, err := filepath.Abs(dir)
		if err != nil {
			return xerrors.Errorf(
				"resolve Terraform module %q directory for agent runtime provenance: %w",
				moduleAddress, err,
			)
		}
		canonicalDir, resolved := resolvedDirectories[absoluteDir]
		if !resolved {
			canonicalDir, err = filepath.EvalSymlinks(absoluteDir)
			if err != nil {
				return xerrors.Errorf(
					"resolve Terraform module %q directory for agent runtime provenance: %w",
					moduleAddress, err,
				)
			}
			resolvedDirectories[absoluteDir] = canonicalDir
		}
		if len(moduleAddressesByDirectory[canonicalDir]) == 0 {
			directories = append(directories, canonicalDir)
		}
		moduleAddressesByDirectory[canonicalDir] = append(
			moduleAddressesByDirectory[canonicalDir], moduleAddress,
		)
	}

	budget := newProvenanceBudget(limits)
	var directoryEntryCount, fileCount, sourceBytes int
	for _, dir := range directories {
		if err := ctx.Err(); err != nil {
			return err
		}
		moduleAddresses := moduleAddressesByDirectory[dir]
		entries, entriesRead, err := readRuntimeSourceFiles(
			ctx,
			dir,
			limits.sourceFiles-fileCount,
			limits.directoryEntries-directoryEntryCount,
		)
		switch {
		case xerrors.Is(err, errRuntimeSourceFileLimit):
			return xerrors.Errorf(
				"agent runtime provenance exceeds the limit of %d Terraform source files",
				limits.sourceFiles,
			)
		case xerrors.Is(err, errRuntimeDirectoryEntryLimit):
			return xerrors.Errorf(
				"agent runtime provenance exceeds the limit of %d Terraform module directory entries",
				limits.directoryEntries,
			)
		case err != nil:
			if err := ctx.Err(); err != nil {
				return err
			}
			return xerrors.Errorf(
				"read Terraform module %q for agent runtime provenance: %w",
				moduleAddresses[0], err,
			)
		}
		directoryEntryCount += entriesRead
		fileCount += len(entries)
		slices.SortStableFunc(entries, func(a, b os.DirEntry) int {
			aOverride := runtimeOverrideFile(a.Name())
			bOverride := runtimeOverrideFile(b.Name())
			switch {
			case aOverride == bOverride:
				return 0
			case aOverride:
				return 1
			default:
				return -1
			}
		})
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			filename := filepath.Join(dir, entry.Name())
			source, limitExceeded, err := readRuntimeFile(
				filename, limits.sourceBytes-sourceBytes,
			)
			if err != nil {
				return xerrors.Errorf(
					"read Terraform source file %q: %w", entry.Name(), err,
				)
			}
			if limitExceeded {
				return xerrors.Errorf(
					"agent runtime provenance exceeds the limit of %d Terraform source bytes",
					limits.sourceBytes,
				)
			}
			sourceBytes += len(source)
			if err := i.indexRuntimeSourceFile(
				ctx,
				filename,
				entry.Name(),
				source,
				moduleAddresses,
				budget,
			); err != nil {
				return err
			}
		}
	}
	return i.analyzeRuntimeSourceExpressions(ctx, budget)
}

func (i *configIndex) indexRuntimeSourceFile(
	ctx context.Context,
	filename string,
	diagnosticName string,
	source []byte,
	moduleAddresses []string,
	budget *provenanceBudget,
) error {
	parser := hclparse.NewParser()
	var file *hcl.File
	var diagnostics hcl.Diagnostics
	jsonSource := strings.HasSuffix(diagnosticName, ".tf.json")
	if jsonSource {
		file, diagnostics = parser.ParseJSON(source, filename)
	} else {
		file, diagnostics = parser.ParseHCL(source, filename)
	}
	if diagnostics.HasErrors() {
		return xerrors.Errorf(
			"parse Terraform source file %q for agent runtime provenance: %s",
			diagnosticName, diagnostics.Error(),
		)
	}
	content, _, diagnostics := file.Body.PartialContent(runtimeSourceSchema)
	if diagnostics.HasErrors() {
		return xerrors.Errorf(
			"read agent runtime expressions from Terraform source file %q: %s",
			diagnosticName, diagnostics.Error(),
		)
	}
	for _, block := range content.Blocks {
		if err := i.indexRuntimeSourceBlock(
			ctx,
			block,
			moduleAddresses,
			source,
			diagnosticName,
			budget,
		); err != nil {
			return err
		}
	}
	return nil
}

func (i *configIndex) indexRuntimeSourceBlock(
	ctx context.Context,
	block *hcl.Block,
	moduleAddresses []string,
	source []byte,
	filename string,
	budget *provenanceBudget,
) error {
	var (
		kind         runtimeExpressionKind
		resourceType string
		name         string
		attributes   hcl.Attributes
		diagnostics  hcl.Diagnostics
	)
	switch block.Type {
	case "locals":
		kind = runtimeExpressionLocal
		attributes, diagnostics = block.Body.JustAttributes()
	case "resource":
		if len(block.Labels) != 2 {
			return xerrors.Errorf(
				"read agent runtime expressions from Terraform source file %q: resource block has invalid labels",
				filename,
			)
		}
		kind = runtimeExpressionResource
		resourceType, name = block.Labels[0], block.Labels[1]
		content, _, contentDiagnostics := block.Body.PartialContent(
			&hcl.BodySchema{Attributes: []hcl.AttributeSchema{
				{Name: "agent_id"},
				{Name: "for_each"},
			}},
		)
		attributes, diagnostics = content.Attributes, contentDiagnostics
	case "module":
		if len(block.Labels) != 1 {
			return xerrors.Errorf(
				"read agent runtime expressions from Terraform source file %q: module block has invalid labels",
				filename,
			)
		}
		kind = runtimeExpressionModuleCall
		name = block.Labels[0]
		attributes, diagnostics = block.Body.JustAttributes()
	case "output":
		if len(block.Labels) != 1 {
			return xerrors.Errorf(
				"read agent runtime expressions from Terraform source file %q: output block has invalid labels",
				filename,
			)
		}
		kind = runtimeExpressionOutput
		name = block.Labels[0]
		content, _, contentDiagnostics := block.Body.PartialContent(
			&hcl.BodySchema{Attributes: []hcl.AttributeSchema{
				{Name: "value", Required: true},
			}},
		)
		attributes, diagnostics = content.Attributes, contentDiagnostics
	default:
		return nil
	}
	if diagnostics.HasErrors() {
		return xerrors.Errorf(
			"read agent runtime expressions from Terraform source file %q: %s",
			filename, diagnostics.Error(),
		)
	}

	for _, attributeName := range slices.Sorted(maps.Keys(attributes)) {
		if err := ctx.Err(); err != nil {
			return err
		}
		expression := attributes[attributeName].Expr
		if strings.HasSuffix(filename, ".tf.json") {
			expression = runtimeJSONExpression(expression, source)
		}
		key := runtimeExpressionKey{
			kind: kind, resourceType: resourceType, name: name,
			attribute: attributeName,
		}
		for _, moduleAddress := range moduleAddresses {
			if err := budget.consumeSourceExpression(); err != nil {
				return err
			}
			key.moduleAddress = moduleAddress
			i.runtimeSourceExpressions[key] = expression
		}
	}
	return nil
}

func readRuntimeSourceFiles(
	ctx context.Context,
	dir string,
	sourceFileLimit int,
	directoryEntryLimit int,
) ([]os.DirEntry, int, error) {
	directory, err := os.Open(dir)
	if err != nil {
		return nil, 0, err
	}
	defer directory.Close()

	var sourceFiles []os.DirEntry
	directoryEntryCount := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		entries, readErr := directory.ReadDir(256)
		for _, entry := range entries {
			isSourceFile := !entry.IsDir() &&
				!strings.HasPrefix(entry.Name(), ".") &&
				(strings.HasSuffix(entry.Name(), ".tf") ||
					strings.HasSuffix(entry.Name(), ".tf.json"))
			if isSourceFile && len(sourceFiles) == sourceFileLimit {
				return nil, 0, errRuntimeSourceFileLimit
			}
			if directoryEntryCount == directoryEntryLimit {
				return nil, 0, errRuntimeDirectoryEntryLimit
			}
			directoryEntryCount++
			if isSourceFile {
				sourceFiles = append(sourceFiles, entry)
			}
		}
		if readErr != nil {
			if xerrors.Is(readErr, io.EOF) {
				break
			}
			return nil, 0, readErr
		}
	}
	slices.SortFunc(sourceFiles, func(a, b os.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return sourceFiles, directoryEntryCount, nil
}

func runtimeOverrideFile(name string) bool {
	return name == "override.tf" || name == "override.tf.json" ||
		strings.HasSuffix(name, "_override.tf") ||
		strings.HasSuffix(name, "_override.tf.json")
}

func (i *configIndex) runtimeModuleDirectories(
	ctx context.Context,
	workdir string,
) (map[string]string, error) {
	return i.runtimeModuleDirectoriesWithLimit(
		ctx, workdir, maxRuntimeSourceBytes,
	)
}

func (i *configIndex) runtimeModuleDirectoriesWithLimit(
	ctx context.Context,
	workdir string,
	manifestByteLimit int,
) (map[string]string, error) {
	directories := map[string]string{"": workdir}
	manifestPath := filepath.Join(workdir, ".terraform", "modules", "modules.json")
	manifestRaw, limitExceeded, err := readRuntimeFile(
		manifestPath, manifestByteLimit,
	)
	if err == nil {
		if limitExceeded {
			return nil, xerrors.Errorf(
				"agent runtime provenance exceeds the limit of %d Terraform module manifest bytes",
				manifestByteLimit,
			)
		}
		var manifest runtimeModulesManifest
		if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
			return nil, xerrors.Errorf("parse Terraform module manifest: %w", err)
		}
		for _, module := range manifest.Modules {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if module == nil {
				continue
			}
			moduleAddress := runtimeModuleAddressForManifestKey(module.Key)
			if _, configured := i.modules[moduleAddress]; configured {
				directories[moduleAddress] = filepath.Join(workdir, module.Dir)
			}
		}
	} else if !xerrors.Is(err, os.ErrNotExist) {
		return nil, xerrors.Errorf("read Terraform module manifest: %w", err)
	}

	for _, moduleAddress := range slices.Sorted(maps.Keys(i.modules)) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if moduleAddress == "" || directories[moduleAddress] != "" {
			continue
		}
		modulePath, err := tfaddr.ParseModulePath(moduleAddress)
		if err != nil {
			return nil, xerrors.Errorf(
				"parse configuration module address %q: %w", moduleAddress, err,
			)
		}
		steps := modulePath.Steps()
		parentAddress := runtimeConfigurationModuleAddress(
			steps[:len(steps)-1],
		)
		parentDir := directories[parentAddress]
		call, ok := i.moduleCalls[moduleCallKey{
			moduleAddress: parentAddress,
			moduleName:    steps[len(steps)-1].Name(),
		}]
		if parentDir == "" || !ok ||
			(!strings.HasPrefix(call.source, "./") &&
				!strings.HasPrefix(call.source, "../")) {
			continue
		}
		directories[moduleAddress] = filepath.Clean(
			filepath.Join(parentDir, call.source),
		)
	}
	return directories, nil
}

func readRuntimeFile(filename string, limit int) ([]byte, bool, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	contents, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, false, err
	}
	if len(contents) > limit {
		return nil, true, nil
	}
	return contents, false, nil
}

func runtimeModuleAddressForManifestKey(key string) string {
	if key == "" {
		return ""
	}
	parts := strings.Split(key, ".")
	for index, part := range parts {
		parts[index] = "module." + part
	}
	return strings.Join(parts, ".")
}

func runtimeConfigurationModuleAddress(steps []tfaddr.ModuleStep) string {
	var address strings.Builder
	for index, step := range steps {
		if index > 0 {
			_ = address.WriteByte('.')
		}
		_, _ = address.WriteString("module.")
		_, _ = address.WriteString(step.Name())
	}
	return address.String()
}

func runtimeJSONExpression(
	expression hcl.Expression,
	source []byte,
) hcl.Expression {
	sourceRange := expression.Range()
	if !sourceRange.CanSliceBytes(source) {
		return expression
	}
	var template string
	if err := json.Unmarshal(sourceRange.SliceBytes(source), &template); err != nil {
		return expression
	}
	start := sourceRange.Start
	start.Byte++
	start.Column++
	parsed, diagnostics := hclsyntax.ParseTemplate(
		[]byte(template), sourceRange.Filename, start,
	)
	if diagnostics.HasErrors() {
		return expression
	}
	return parsed
}
