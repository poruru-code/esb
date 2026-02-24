package deployops

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var mavenRunCommandPattern = regexp.MustCompile(`(^|&&|\|\||;)[[:space:]]*mvn([[:space:]]|$)`)

var mavenWrapperRunCommandPattern = regexp.MustCompile(`(^|&&|\|\||;)[[:space:]]*\./mvnw([[:space:]]|$)`)

func resolveFunctionBuildDockerfile(
	dockerfile string,
	runtime *RuntimeObservation,
	noCache bool,
	runner CommandRunner,
	resolvedMavenShimImages map[string]string,
) (string, func(), error) {
	data, err := os.ReadFile(dockerfile)
	if err != nil {
		return "", nil, fmt.Errorf("read dockerfile %s: %w", dockerfile, err)
	}
	rewritten, changed := rewriteDockerfileForBuild(
		string(data),
		resolveHostFunctionRegistry(),
		resolveRegistryAliases(),
		resolveLambdaBaseTag(runtime),
	)
	rewritten, mavenChanged, err := rewriteDockerfileForMavenShim(
		rewritten,
		func(baseRef string) (string, error) {
			return ensureMavenShimImage(baseRef, noCache, runner, resolvedMavenShimImages)
		},
	)
	if err != nil {
		return "", nil, err
	}
	if mavenChanged {
		changed = true
	}
	if !changed {
		return dockerfile, func() {}, nil
	}

	tmpPath := dockerfile + ".artifact.build"
	if err := os.WriteFile(tmpPath, []byte(rewritten), 0o644); err != nil {
		return "", nil, fmt.Errorf("write temporary dockerfile %s: %w", tmpPath, err)
	}
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}
	return tmpPath, cleanup, nil
}

func rewriteDockerfileForBuild(content, hostRegistry string, registryAliases []string, lambdaBaseTag string) (string, bool) {
	lines := strings.Split(content, "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trimmed), "from ") {
			continue
		}
		parts := strings.Fields(trimmed)
		if len(parts) < 2 {
			continue
		}
		refIndex := fromImageTokenIndex(parts)
		if refIndex < 0 {
			continue
		}
		rewrittenRef, rewritten := rewriteDockerfileFromRef(parts[refIndex], hostRegistry, registryAliases, lambdaBaseTag)
		if !rewritten {
			continue
		}
		parts[refIndex] = rewrittenRef
		indentLen := len(line) - len(strings.TrimLeft(line, " \t"))
		indent := line[:indentLen]
		lines[i] = indent + strings.Join(parts, " ")
		changed = true
	}
	if !changed {
		return content, false
	}
	return strings.Join(lines, "\n"), true
}

func rewriteDockerfileForMavenShim(
	content string,
	resolveShim func(baseRef string) (string, error),
) (string, bool, error) {
	lines := strings.Split(content, "\n")
	changed := false
	sawMavenRun := false
	rewrittenMavenBase := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lowerTrimmed := strings.ToLower(trimmed)
		if strings.HasPrefix(lowerTrimmed, "run ") {
			command := strings.TrimSpace(trimmed[4:])
			if mavenRunCommandPattern.MatchString(command) || mavenWrapperRunCommandPattern.MatchString(command) {
				sawMavenRun = true
			}
			continue
		}
		if !strings.HasPrefix(lowerTrimmed, "from ") {
			continue
		}
		parts := strings.Fields(trimmed)
		if len(parts) < 2 {
			continue
		}
		refIndex := fromImageTokenIndex(parts)
		if refIndex < 0 {
			continue
		}
		baseRef := strings.TrimSpace(parts[refIndex])
		if !isMavenBaseRef(baseRef) {
			continue
		}
		shimRef, err := resolveShim(baseRef)
		if err != nil {
			return "", false, err
		}
		parts[refIndex] = shimRef
		indentLen := len(line) - len(strings.TrimLeft(line, " \t"))
		indent := line[:indentLen]
		lines[i] = indent + strings.Join(parts, " ")
		rewrittenMavenBase = true
		changed = true
	}

	if sawMavenRun && !rewrittenMavenBase {
		return "", false, fmt.Errorf(
			"maven run command detected but no maven base stage is rewriteable; use 'FROM maven:...' (or equivalent maven repo) in Dockerfile",
		)
	}
	if !changed {
		return content, false, nil
	}
	return strings.Join(lines, "\n"), true, nil
}

func isMavenBaseRef(imageRef string) bool {
	ref := strings.TrimSpace(imageRef)
	if ref == "" {
		return false
	}
	withoutDigest := strings.SplitN(ref, "@", 2)[0]
	slash := strings.LastIndex(withoutDigest, "/")
	colon := strings.LastIndex(withoutDigest, ":")
	repo := withoutDigest
	if colon > slash {
		repo = withoutDigest[:colon]
	}
	lastSegment := repo
	if slash >= 0 && slash+1 < len(repo) {
		lastSegment = repo[slash+1:]
	}
	return lastSegment == "maven"
}

func rewriteDockerfileFromRef(ref, hostRegistry string, registryAliases []string, lambdaBaseTag string) (string, bool) {
	current := strings.TrimSpace(ref)
	if current == "" {
		return ref, false
	}
	rewritten := current
	changed := false

	if baseRef, refChanged := rewriteLambdaBaseRefForBuild(rewritten); refChanged {
		rewritten = baseRef
		changed = true
	} else if hostRegistry != "" {
		if baseRef, refChanged := rewriteRegistryAlias(rewritten, hostRegistry, registryAliases); refChanged {
			rewritten = baseRef
			changed = true
		}
	}

	if lambdaBaseTag == "" {
		return rewritten, changed
	}
	next, tagChanged := rewriteLambdaBaseTag(rewritten, lambdaBaseTag)
	if tagChanged {
		rewritten = next
		changed = true
	}
	return rewritten, changed
}

func rewriteLambdaBaseTag(imageRef, tag string) (string, bool) {
	trimmedTag := strings.TrimSpace(tag)
	if trimmedTag == "" {
		return imageRef, false
	}
	withoutDigest := strings.SplitN(strings.TrimSpace(imageRef), "@", 2)[0]
	if withoutDigest == "" {
		return imageRef, false
	}

	slash := strings.LastIndex(withoutDigest, "/")
	colon := strings.LastIndex(withoutDigest, ":")
	repo := withoutDigest
	if colon > slash {
		repo = withoutDigest[:colon]
	}
	lastSegment := repo
	if slash >= 0 && slash+1 < len(repo) {
		lastSegment = repo[slash+1:]
	}
	if lastSegment != "esb-lambda-base" {
		return imageRef, false
	}

	// Keep explicit (non-latest) tags authored in artifact Dockerfiles.
	// Runtime tag override is only applied to floating latest references.
	if colon > slash {
		currentTag := withoutDigest[colon+1:]
		if currentTag != "" && currentTag != "latest" {
			return imageRef, false
		}
	}
	return repo + ":" + trimmedTag, true
}

func fromImageTokenIndex(parts []string) int {
	for i := 1; i < len(parts); i++ {
		token := strings.TrimSpace(parts[i])
		if token == "" {
			continue
		}
		if strings.HasPrefix(token, "--") {
			continue
		}
		return i
	}
	return -1
}
