package grabanaclistarter

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"github.com/K-Phoen/grabana"
	"github.com/K-Phoen/grabana/dashboard"
	"github.com/K-Phoen/grabana/datasource/prometheus"
	"github.com/cryptvault-cloud/helper"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"
)

type CliValues = string

const (
	CliServer            CliValues = "server"
	CliApiKey            CliValues = "apikey"
	CliFolderName        CliValues = "foldername"
	CliYamlTargetFile    CliValues = "file"
	CliDevDatasourceName string    = "datasource_name"
)

//go:embed prometheus.yml.tmpl
var prometheusTmpl []byte

type Option func(runner *Runner, app *cli.App) error

type DashboardCreator func(folderName string, c *cli.Context) ([]dashboard.Builder, error)

func DashboardBuilder(d DashboardCreator) Option {
	return func(runner *Runner, app *cli.App) error {
		if runner.Dashboard != nil {
			return fmt.Errorf("Dashboard already set")
		}
		runner.Dashboard = d
		return nil
	}
}

func DefaultDevRunDataSource(value string) Option {
	return func(runner *Runner, app *cli.App) error {
		for _, c := range app.Commands {
			if c.Name == "dev" {
				for _, sc := range c.Subcommands {
					if sc.Name == "run" {
						for _, f := range sc.Flags {
							if helper.Includes(f.Names(), func(name string) bool { return name == CliDevDatasourceName }) {
								strFlag, ok := f.(*cli.StringFlag)
								if !ok {
									return fmt.Errorf("Oh shit something big goes wrong")
								}
								strFlag.Value = value
							}
						}
					}
				}

			}
		}

		return nil
	}
}

func DefaultDashboardCliFlagValue(key CliValues, value string) Option {
	return func(runner *Runner, app *cli.App) error {
		for _, c := range app.Commands {
			if c.Name == "dashboard" {
				for _, f := range c.Flags {
					if helper.Includes(f.Names(), func(name string) bool { return name == key }) {
						strFlag, ok := f.(*cli.StringFlag)
						if !ok {
							return fmt.Errorf("Oh shit something big goes wrong")
						}
						strFlag.Value = value
					}
				}
			}
		}

		return nil
	}
}

type Runner struct {
	Client    *grabana.Client
	Ctx       context.Context
	Dashboard DashboardCreator
}

func GetFlagEnvByFlagName(flagName, appName string) string {
	return fmt.Sprintf("%s_%s", appName, strings.ToUpper(flagName))
}

func NewCli(appName string, options ...Option) (*cli.App, error) {
	runner := Runner{}
	app := &cli.App{
		Usage: "vault-server",

		Commands: []*cli.Command{
			{
				Name:   "dashboard",
				Usage:  "To apply destroy and plan current dashboard",
				Before: runner.Before,
				Subcommands: []*cli.Command{
					{
						Name:   "apply",
						Action: runner.Apply,
						Usage:  "Upload Dashboard to target configuration",
					},
					{
						Name:   "destroy",
						Action: runner.Destroy,
						Usage:  "Remove Dashboard from target configuration",
					},
					{
						Name:   "plan",
						Action: runner.Plan,
						Usage:  "Upload Dashboard to target configuration",
					},
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    CliFolderName,
						EnvVars: []string{GetFlagEnvByFlagName(CliFolderName, appName)},
						Usage:   "GrafanaFolder to create dashboards",
					},
					&cli.StringFlag{
						Name:    CliServer,
						EnvVars: []string{GetFlagEnvByFlagName(CliServer, appName)},
						Usage:   "grafana url",
					},
					&cli.StringFlag{
						Name:     CliApiKey,
						EnvVars:  []string{GetFlagEnvByFlagName(CliApiKey, appName)},
						Required: true,
						Usage:    "grafana api key",
					},
				},
			},
			{
				Name:   "toYaml",
				Action: runner.ToYaml,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    CliYamlTargetFile,
						EnvVars: []string{GetFlagEnvByFlagName(CliYamlTargetFile, appName)},
						Value:   "target.yml",
						Usage:   "file to save yaml",
					},
				},
			},
			{
				Name: "dev",
				Subcommands: []*cli.Command{
					{
						Name:   "init",
						Usage:  "Generate template prometheus folder/file to configure scrape stuff for local dev server (DO NOT move this files and start dev server from same path)",
						Action: runner.InitDev,
					},
					{
						Name:  "run",
						Usage: "Start DEV prometheus and grafana",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     CliDevDatasourceName,
								EnvVars:  []string{GetFlagEnvByFlagName(CliDevDatasourceName, appName)},
								Aliases:  []string{"datasource"},
								Required: true,
							},
						},
						After: runner.startDev,
					},
				},
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
	board, err := r.Dashboard(c.String(CliFolderName), c)
	if err != nil {
		return err
	}

	err = errors.Join(nil)
	for _, b := range board {
		tmpErr := r.Client.DeleteDashboard(r.Ctx, b.Internal().UID)
		if tmpErr != nil {
			err = errors.Join(err, fmt.Errorf("Error by %s: %w", b.Internal().UID, tmpErr))
		}
	}
	return err
}

func (r *Runner) Apply(c *cli.Context) error {
	folder, err := r.Client.FindOrCreateFolder(r.Ctx, c.String(CliFolderName))
	if err != nil {
		return fmt.Errorf("Could not find or create folder: %w\n", err)
	}
	board, err := r.Dashboard(c.String(CliFolderName), c)
	if err != nil {
		return err
	}
	err = errors.Join(nil)
	for _, b := range board {
		dash, tmpErr := r.Client.UpsertDashboard(r.Ctx, folder, b)
		if err != nil {
			err = errors.Join(err, fmt.Errorf("Could not create dashboard: %w\n", tmpErr))
		} else {
			fmt.Printf("The deed is done:\n%s\n", c.String(CliServer)+dash.URL)
		}
	}
	return err
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
	board, err := r.Dashboard(c.String(CliFolderName), c)
	if err != nil {
		return err
	}
	err = errors.Join(nil)

	for _, b := range board {
		json, tmpErr := b.MarshalIndentJSON()
		if err != nil {
			err = errors.Join(err, fmt.Errorf("Error by %s: %w", b.Internal().UID, tmpErr))
		}
		fmt.Println(string(json))
	}

	return err
}

// EnsureDir checks if given directory exist, creates if not
func EnsureDir(dir string) error {
	if !DirExist(dir) {
		err := os.Mkdir(dir, 0755)
		if err != nil {
			return err
		}
	}
	return nil
}

// DirExist checks if directory exist
func DirExist(dir string) bool {
	_, err := os.Stat(dir)
	if err == nil {
		return true
	}
	return !os.IsNotExist(err)
}
func (r *Runner) InitDev(c *cli.Context) error {

	err := EnsureDir("./prometheus")
	if err != nil {
		return err
	}
	if !DirExist("./prometheus/prometheus.yml") {
		err = os.WriteFile("./prometheus/prometheus.yml", prometheusTmpl, os.ModePerm)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) startDev(c *cli.Context) error {
	err := r.InitDev(c)
	if err != nil {
		return err
	}
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	ctx := context.Background()
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	networkName := "grabana_dev"
	newNetwork, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		ProviderType: testcontainers.ProviderDocker,
		NetworkRequest: testcontainers.NetworkRequest{
			Name:           networkName,
			CheckDuplicate: true,
		},
	})
	if err != nil {
		return err
	}
	defer func() {
		if err := newNetwork.Remove(ctx); err != nil {
			panic(err)
		}
	}()

	prometheusContainerName := "prometheus_" + uuid.New().String()
	prometheusPort := "9090/tcp"
	req := testcontainers.ContainerRequest{
		Name:         prometheusContainerName,
		Image:        "prom/prometheus:latest",
		ExposedPorts: []string{prometheusPort},

		Mounts: testcontainers.ContainerMounts{
			testcontainers.BindMount(path.Join(pwd, "prometheus"), "/etc/prometheus"),
		},
		Privileged: true,
		Networks:   []string{networkName},
		WaitingFor: wait.ForListeningPort(nat.Port(prometheusPort)),
	}

	prometheusC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return err
	}
	defer func() {
		if err := prometheusC.Terminate(ctx); err != nil {
			panic(err)
		}
	}()
	grafanaPort := "3000/tcp"
	req2 := testcontainers.ContainerRequest{
		Image:        "grafana/grafana:latest",
		ExposedPorts: []string{grafanaPort},

		Mounts: testcontainers.ContainerMounts{
			testcontainers.BindMount(path.Join(pwd, "prometheus"), "/etc/prometheus"),
		},
		Networks:   []string{networkName},
		WaitingFor: wait.ForListeningPort(nat.Port(grafanaPort)),
	}
	grafanaC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req2,
		Started:          true,
	})
	if err != nil {
		return err
	}
	defer func() {
		if err := grafanaC.Terminate(ctx); err != nil {
			panic(err)
		}
	}()

	grafanaUrl, err := grafanaC.PortEndpoint(ctx, nat.Port(grafanaPort), "http")
	if err != nil {
		return err
	}
	prometheusUrl, err := prometheusC.PortEndpoint(ctx, nat.Port(prometheusPort), "http")
	if err != nil {
		return err
	}
	client := grabana.NewClient(&http.Client{}, grafanaUrl, grabana.WithBasicAuth("admin", "admin"))
	prometheusDatasource, err := prometheus.New(c.String(CliDevDatasourceName), fmt.Sprintf("http://%s:9090", prometheusContainerName))
	if err != nil {
		return err
	}
	err = client.UpsertDatasource(r.Ctx, prometheusDatasource)
	if err != nil {
		return err
	}
	apiKey, err := client.CreateAPIKey(r.Ctx, grabana.CreateAPIKeyRequest{
		Name:          "Grabana_debug",
		Role:          grabana.AdminRole,
		SecondsToLive: 0,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Prometheus endpoint: %s \n", prometheusUrl)
	fmt.Printf("Grafana endpoint: %s \n", grafanaUrl)
	fmt.Printf("\tGrafana user: admin \n")
	fmt.Printf("\tGrafana password: admin \n")
	fmt.Printf("\tPrometheus Datasourcename: %s\n", c.String(CliDevDatasourceName))
	fmt.Printf("\tApi key: %s \n", apiKey)
	fmt.Printf("Simple run\n go run . dashboard --server %s --apikey %s apply\n", grafanaUrl, apiKey)
	<-done
	return nil
}
