package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/antonmedv/expr"
	"github.com/goccy/go-yaml"
	"github.com/k1LoW/duration"
	"github.com/k1LoW/expand"
	"github.com/k1LoW/octocov/gh"
	"github.com/k1LoW/octocov/report"
)

const defaultBadgesDatastore = "local://reports"
const defaultReportsDatastore = "local://reports"

const (
	// https://github.com/badges/shields/blob/7d452472defa0e0bd71d6443393e522e8457f856/badge-maker/lib/color.js#L8-L12
	green       = "#97CA00"
	yellowgreen = "#A4A61D"
	yellow      = "#DFB317"
	orange      = "#FE7D37"
	red         = "#E05D44"
)

var DefaultConfigFilePaths = []string{".octocov.yml", "octocov.yml"}

type Config struct {
	Repository        string                   `yaml:"repository"`
	Coverage          *ConfigCoverage          `yaml:"coverage"`
	CodeToTestRatio   *ConfigCodeToTestRatio   `yaml:"codeToTestRatio,omitempty"`
	TestExecutionTime *ConfigTestExecutionTime `yaml:"testExecutionTime,omitempty"`
	Report            *ConfigReport            `yaml:"report,omitempty"`
	Central           *ConfigCentral           `yaml:"central,omitempty"`
	Push              *ConfigPush              `yaml:"push,omitempty"`
	Comment           *ConfigComment           `yaml:"comment,omitempty"`
	Diff              *ConfigDiff              `yaml:"diff,omitempty"`
	GitRoot           string                   `yaml:"-"`
	// working directory
	wd string
	// config file path
	path string
	gh   *gh.Gh
}

type ConfigCoverage struct {
	Path       string              `yaml:"path,omitempty"`
	Badge      ConfigCoverageBadge `yaml:"badge,omitempty"`
	Acceptable string              `yaml:"acceptable,omitempty"`
}

type ConfigCoverageBadge struct {
	Path string `yaml:"path,omitempty"`
}

type ConfigCodeToTestRatio struct {
	Code       []string                   `yaml:"code"`
	Test       []string                   `yaml:"test"`
	Badge      ConfigCodeToTestRatioBadge `yaml:"badge,omitempty"`
	Acceptable string                     `yaml:"acceptable,omitempty"`
}

type ConfigCodeToTestRatioBadge struct {
	Path string `yaml:"path,omitempty"`
}

type ConfigTestExecutionTime struct {
	Badge      ConfigTestExecutionTimeBadge `yaml:"badge,omitempty"`
	Acceptable string                       `yaml:"acceptable,omitempty"`
	Steps      []string                     `yaml:"steps,omitempty"`
}

type ConfigTestExecutionTimeBadge struct {
	Path string `yaml:"path,omitempty"`
}

type ConfigCentral struct {
	Enable  *bool                `yaml:"enable,omitempty"`
	Root    string               `yaml:"root"`
	Reports ConfigCentralReports `yaml:"reports"`
	Badges  ConfigCentralBadges  `yaml:"badges"`
	Push    *ConfigPush          `yaml:"push"`
	If      string               `yaml:"if,omitempty"`
}

type ConfigCentralReports struct {
	Datastores []string `yaml:"datastores"`
}

type ConfigCentralBadges struct {
	Datastores []string `yaml:"datastores"`
}

type ConfigPush struct {
	Enable *bool  `yaml:"enable,omitempty"`
	If     string `yaml:"if,omitempty"`
}

type ConfigComment struct {
	Enable         *bool  `yaml:"enable,omitempty"`
	HideFooterLink bool   `yaml:"hideFooterLink"`
	If             string `yaml:"if,omitempty"`
}

type ConfigDiff struct {
	Path       string   `yaml:"path,omitempty"`
	Datastores []string `yaml:"datastores,omitempty"`
	If         string   `yaml:"if,omitempty"`
}

func New() *Config {
	wd, _ := os.Getwd()
	return &Config{
		wd: wd,
	}
}

func (c *Config) Getwd() string {
	return c.wd
}

func (c *Config) Setwd(path string) {
	c.wd = path
}

func (c *Config) Load(path string) error {
	if path == "" {
		for _, p := range DefaultConfigFilePaths {
			if f, err := os.Stat(filepath.Join(c.wd, p)); err == nil && !f.IsDir() {
				if path != "" {
					return fmt.Errorf("duplicate config file [%s, %s]", path, p)
				}
				path = p
			}
		}
	}
	if path == "" {
		return nil
	}
	c.path = filepath.Join(c.wd, path)
	buf, err := os.ReadFile(filepath.Clean(c.path))
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(expand.ExpandenvYAMLBytes(buf), c); err != nil {
		return err
	}
	return nil
}

func (c *Config) Root() string {
	if c.path != "" {
		return filepath.Dir(c.path)
	}
	return c.wd
}

func (c *Config) Loaded() bool {
	return c.path != ""
}

func (c *Config) Acceptable(r, rPrev *report.Report) error {
	if err := c.CoverageConfigReady(); err == nil {
		if err := coverageAcceptable(r.CoveragePercent(), c.Coverage.Acceptable); err != nil {
			return err
		}
	}

	if err := c.CodeToTestRatioConfigReady(); err == nil {
		if err := codeToTestRatioAcceptable(r.CodeToTestRatioRatio(), c.CodeToTestRatio.Acceptable); err != nil {
			return err
		}
	}

	if err := c.TestExecutionTimeConfigReady(); err == nil {
		if err := testExecutionTimeAcceptable(r.TestExecutionTimeNano(), c.TestExecutionTime.Acceptable); err != nil {
			return err
		}
	}

	return nil
}

func coverageAcceptable(cov float64, cond string) error {
	if cond == "" {
		return nil
	}
	a, err := strconv.ParseFloat(strings.TrimSuffix(cond, "%"), 64)
	if err != nil {
		return err
	}
	if cov < a {
		return fmt.Errorf("code coverage is %.1f%%, which is below the accepted %.1f%%", cov, a)
	}
	return nil
}

func codeToTestRatioAcceptable(ratio float64, cond string) error {
	if cond == "" {
		return nil
	}
	a, err := strconv.ParseFloat(strings.TrimPrefix(cond, "1:"), 64)
	if err != nil {
		return err
	}
	if ratio < a {
		return fmt.Errorf("code to test ratio is 1:%.1f, which is below the accepted 1:%.1f", ratio, a)
	}
	return nil
}

func testExecutionTimeAcceptable(t float64, cond string) error {
	if cond == "" {
		return nil
	}
	a, err := duration.Parse(cond)
	if err != nil {
		return err
	}
	if t > float64(a) {
		return fmt.Errorf("test execution time is %v, which is above the accepted %v", time.Duration(t), a)
	}
	return nil
}

func (c *Config) CoverageColor(cover float64) string {
	switch {
	case cover >= 80.0:
		return green
	case cover >= 60.0:
		return yellowgreen
	case cover >= 40.0:
		return yellow
	case cover >= 20.0:
		return orange
	default:
		return red
	}
}

func (c *Config) CodeToTestRatioColor(ratio float64) string {
	switch {
	case ratio >= 1.2:
		return green
	case ratio >= 1.0:
		return yellowgreen
	case ratio >= 0.8:
		return yellow
	case ratio >= 0.6:
		return orange
	default:
		return red
	}
}

func (c *Config) TestExecutionTimeColor(d time.Duration) string {
	switch {
	case d < 5*time.Minute:
		return green
	case d < 10*time.Minute:
		return yellowgreen
	case d < 15*time.Minute:
		return yellow
	case d < 20*time.Minute:
		return orange
	default:
		return red
	}
}

func (c *Config) CheckIf(cond string) (bool, error) {
	if cond == "" {
		return true, nil
	}
	e, err := gh.DecodeGitHubEvent()
	if err != nil {
		return false, err
	}
	if c.Repository == "" {
		return false, fmt.Errorf("env %s is not set", "GITHUB_REPOSITORY")
	}
	ctx := context.Background()
	repo, err := gh.Parse(c.Repository)
	if err != nil {
		return false, err
	}
	if c.gh == nil {
		g, err := gh.New()
		if err != nil {
			return false, err
		}
		c.gh = g
	}
	defaultBranch, err := c.gh.GetDefaultBranch(ctx, repo.Owner, repo.Repo)
	if err != nil {
		return false, err
	}
	isDefaultBranch := false
	if b, err := c.gh.DetectCurrentBranch(ctx); err == nil {
		if b == defaultBranch {
			isDefaultBranch = true
		}
	}

	isPullRequest := false
	if _, err := c.gh.DetectCurrentPullRequestNumber(ctx, repo.Owner, repo.Repo); err == nil {
		isPullRequest = true
	}
	now := time.Now()
	variables := map[string]interface{}{
		"year":    now.UTC().Year(),
		"month":   now.UTC().Month(),
		"day":     now.UTC().Day(),
		"hour":    now.UTC().Hour(),
		"weekday": int(now.UTC().Weekday()),
		"github": map[string]interface{}{
			"event_name": e.Name,
			"event":      e.Payload,
		},
		"env":               envMap(),
		"is_default_branch": isDefaultBranch,
		"is_pull_request":   isPullRequest,
	}
	ok, err := expr.Eval(fmt.Sprintf("(%s) == true", cond), variables)
	if err != nil {
		return false, err
	}
	return ok.(bool), nil
}

func envMap() map[string]string {
	m := map[string]string{}
	for _, kv := range os.Environ() {
		if !strings.Contains(kv, "=") {
			continue
		}
		parts := strings.SplitN(kv, "=", 2)
		k := parts[0]
		if len(parts) < 2 {
			m[k] = ""
			continue
		}
		m[k] = parts[1]
	}
	return m
}
