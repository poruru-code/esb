// Where: cli/internal/workflows/project.go
// What: Project workflows for list/recent/use/remove/register.
// Why: Move project orchestration out of the CLI adapter.
package workflows

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// ProjectInfo represents a project entry for list output.
type ProjectInfo struct {
	Name   string
	Active bool
}

// ProjectListRequest captures inputs for listing projects.
type ProjectListRequest struct {
	Config config.GlobalConfig
}

// ProjectListResult contains the listed projects.
type ProjectListResult struct {
	Projects []ProjectInfo
}

// ProjectListWorkflow lists registered projects.
type ProjectListWorkflow struct{}

// NewProjectListWorkflow constructs a ProjectListWorkflow.
func NewProjectListWorkflow() ProjectListWorkflow {
	return ProjectListWorkflow{}
}

// Run executes the project list workflow.
func (w ProjectListWorkflow) Run(req ProjectListRequest) (ProjectListResult, error) {
	if len(req.Config.Projects) == 0 {
		return ProjectListResult{}, nil
	}

	names := make([]string, 0, len(req.Config.Projects))
	for name := range req.Config.Projects {
		names = append(names, name)
	}
	sort.Strings(names)

	appState, _ := state.ResolveAppState(state.AppStateOptions{
		ProjectEnv: envutil.GetHostEnv(constants.HostSuffixProject),
		Projects:   req.Config.Projects,
	})
	activeProject := appState.ActiveProject

	projects := make([]ProjectInfo, 0, len(names))
	for _, name := range names {
		projects = append(projects, ProjectInfo{
			Name:   name,
			Active: name == activeProject,
		})
	}

	return ProjectListResult{Projects: projects}, nil
}

// ProjectRecentRequest captures inputs for listing recent projects.
type ProjectRecentRequest struct {
	Config config.GlobalConfig
}

// ProjectRecentResult contains recent projects.
type ProjectRecentResult struct {
	Projects []RecentProject
}

// ProjectRecentWorkflow lists projects by recent usage.
type ProjectRecentWorkflow struct{}

// NewProjectRecentWorkflow constructs a ProjectRecentWorkflow.
func NewProjectRecentWorkflow() ProjectRecentWorkflow {
	return ProjectRecentWorkflow{}
}

// Run executes the project recent workflow.
func (w ProjectRecentWorkflow) Run(req ProjectRecentRequest) (ProjectRecentResult, error) {
	return ProjectRecentResult{Projects: SortProjectsByRecent(req.Config)}, nil
}

// ProjectUseRequest captures inputs for switching projects.
type ProjectUseRequest struct {
	ProjectName      string
	GlobalConfig     config.GlobalConfig
	GlobalConfigPath string
	Now              time.Time
}

// ProjectUseWorkflow updates last-used metadata.
type ProjectUseWorkflow struct{}

// NewProjectUseWorkflow constructs a ProjectUseWorkflow.
func NewProjectUseWorkflow() ProjectUseWorkflow {
	return ProjectUseWorkflow{}
}

// Run executes the project use workflow.
func (w ProjectUseWorkflow) Run(req ProjectUseRequest) error {
	if strings.TrimSpace(req.GlobalConfigPath) == "" {
		return errors.New("global config path not available")
	}
	cfg := normalizeGlobalConfig(req.GlobalConfig)
	entry := cfg.Projects[req.ProjectName]
	entry.LastUsed = req.Now.Format(time.RFC3339)
	cfg.Projects[req.ProjectName] = entry
	return config.SaveGlobalConfig(req.GlobalConfigPath, cfg)
}

// ProjectRemoveRequest captures inputs for removing projects.
type ProjectRemoveRequest struct {
	ProjectName      string
	GlobalConfig     config.GlobalConfig
	GlobalConfigPath string
}

// ProjectRemoveWorkflow removes a project from global config.
type ProjectRemoveWorkflow struct{}

// NewProjectRemoveWorkflow constructs a ProjectRemoveWorkflow.
func NewProjectRemoveWorkflow() ProjectRemoveWorkflow {
	return ProjectRemoveWorkflow{}
}

// Run executes the project remove workflow.
func (w ProjectRemoveWorkflow) Run(req ProjectRemoveRequest) error {
	if strings.TrimSpace(req.GlobalConfigPath) == "" {
		return errors.New("global config path not available")
	}
	cfg := normalizeGlobalConfig(req.GlobalConfig)
	delete(cfg.Projects, req.ProjectName)
	return config.SaveGlobalConfig(req.GlobalConfigPath, cfg)
}

// ProjectRegisterRequest captures inputs for registering projects.
type ProjectRegisterRequest struct {
	GeneratorPath string
	Now           time.Time
}

// ProjectRegisterWorkflow registers a project into global config.
type ProjectRegisterWorkflow struct{}

// NewProjectRegisterWorkflow constructs a ProjectRegisterWorkflow.
func NewProjectRegisterWorkflow() ProjectRegisterWorkflow {
	return ProjectRegisterWorkflow{}
}

// Run executes the project register workflow.
func (w ProjectRegisterWorkflow) Run(req ProjectRegisterRequest) error {
	if strings.TrimSpace(req.GeneratorPath) == "" {
		return errors.New("generator path is required")
	}

	absPath, err := filepath.Abs(req.GeneratorPath)
	if err != nil {
		return err
	}
	projectDir := filepath.Dir(absPath)

	cfg, err := config.LoadGeneratorConfig(absPath)
	if err != nil {
		return err
	}

	projectName := strings.TrimSpace(cfg.App.Name)
	if projectName == "" {
		projectName = filepath.Base(projectDir)
	}

	globalPath, err := config.GlobalConfigPath()
	if err != nil {
		return err
	}
	globalCfg, err := loadGlobalConfig(globalPath)
	if err != nil {
		return err
	}

	updated := normalizeGlobalConfig(globalCfg)
	entry := updated.Projects[projectName]
	entry.Path = projectDir
	entry.LastUsed = req.Now.Format(time.RFC3339)
	updated.Projects[projectName] = entry
	return config.SaveGlobalConfig(globalPath, updated)
}

func loadGlobalConfig(path string) (config.GlobalConfig, error) {
	cfg, err := config.LoadGlobalConfig(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config.DefaultGlobalConfig(), nil
		}
		return config.GlobalConfig{}, err
	}
	return normalizeGlobalConfig(cfg), nil
}
