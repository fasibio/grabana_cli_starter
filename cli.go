package grabanaclistarter

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/K-Phoen/grabana"
	"github.com/K-Phoen/grabana/dashboard"
	"github.com/cryptvault-cloud/helper"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"
)

type CliValues = string

const (
	CliServer         CliValues = "server"
	CliApiKey         CliValues = "apikey"
	CliFolderName     CliValues = "foldername"
	CliYamlTargetFile CliValues = "file"
)

type Option func(runner *Runner, app *cli.App) error

func DashboardBuilder(board *dashboard.Builder) Option {
	return func(runner *Runner, app *cli.App) error {
		if runner.Dashboard != nil {
			return fmt.Errorf("Dashboard already set")
		}
		runner.Dashboard = board
		return nil
	}
}

func DefaultCliFlagValue(key CliValues, value string) Option {
	return func(runner *Runner, app *cli.App) error {
		for _, f := range app.Flags {
			if helper.Includes(f.Names(), func(name string) bool { return name == key }) {
				strFlag, ok := f.(*cli.StringFlag)
				if !ok {
					return fmt.Errorf("Oh shit something big goes wrong")
				}
				strFlag.Value = value
			}
		}
		return nil
	}
}

type Runner struct {
	Client    *grabana.Client
	Ctx       context.Context
	Dashboard *dashboard.Builder
}

func getFlagEnvByFlagName(flagName, appName string) string {
	return fmt.Sprintf("%s_%s", appName, strings.ToUpper(flagName))
}

func NewCli(appName string, options ...Option) (*cli.App, error) {
	runner := Runner{}
	app := &cli.App{
		Usage:  "vault-server",
		Before: runner.Before,
		Commands: []*cli.Command{
			{
				Name:   "apply",
				Action: runner.Apply,
			},
			{
				Name:   "destroy",
				Action: runner.Destroy,
			},
			{
				Name:   "plan",
				Action: runner.Plan,
			},
			{
				Name:   "toYaml",
				Action: runner.ToYaml,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    CliYamlTargetFile,
						EnvVars: []string{getFlagEnvByFlagName(CliYamlTargetFile, appName)},
						Value:   "target.yml",
						Usage:   "file to save yaml",
					},
				},
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    CliFolderName,
				EnvVars: []string{getFlagEnvByFlagName(CliFolderName, appName)},
				Usage:   "GrafanaFolder to create dashboards",
			},
			&cli.StringFlag{
				Name:    CliServer,
				EnvVars: []string{getFlagEnvByFlagName(CliServer, appName)},
				Usage:   "grafana url",
			},
			&cli.StringFlag{
				Name:     CliApiKey,
				EnvVars:  []string{getFlagEnvByFlagName(CliApiKey, appName)},
				Required: true,
				Usage:    "grafana api key",
			},
		},
	}
	for _, o := range options {
		err := o(&runner, app)
		if err != nil {
			return nil, err
		}
	}
	return app, nil
}

func (r *Runner) Before(c *cli.Context) error {

	r.Ctx = context.Background()
	r.Client = grabana.NewClient(&http.Client{}, c.String(CliServer), grabana.WithAPIToken(c.String(CliApiKey)))
	return nil
}

func (r *Runner) Destroy(c *cli.Context) error {
	return r.Client.DeleteDashboard(r.Ctx, r.Dashboard.Internal().UID)
}

func (r *Runner) Apply(c *cli.Context) error {
	folder, err := r.Client.FindOrCreateFolder(r.Ctx, c.String(CliFolderName))
	if err != nil {
		return fmt.Errorf("Could not find or create folder: %w\n", err)
	}

	dash, err := r.Client.UpsertDashboard(r.Ctx, folder, *r.Dashboard)
	if err != nil {
		return fmt.Errorf("Could not create dashboard: %w\n", err)
	}

	fmt.Printf("The deed is done:\n%s\n", c.String(CliServer)+dash.URL)
	return nil
}
func (r *Runner) ToYaml(c *cli.Context) error {

	filepath := c.String(CliYamlTargetFile)
	f, err := os.Create(filepath)
	if err != nil {
		return err
	}
	encoder := yaml.NewEncoder(f)

	return encoder.Encode(&r.Dashboard)
}

func (r *Runner) Plan(c *cli.Context) error {
	json, err := r.Dashboard.MarshalIndentJSON()
	if err != nil {
		return err
	}

	fmt.Println(string(json))
	return nil
}
