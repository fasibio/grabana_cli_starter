package recordingrules

//Work in process draft

import (
	"crypto/md5"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/K-Phoen/grabana/gauge"
	"github.com/K-Phoen/grabana/graph"
	"github.com/K-Phoen/grabana/heatmap"
	"github.com/K-Phoen/grabana/singlestat"
	"github.com/K-Phoen/grabana/stat"
	"github.com/K-Phoen/grabana/table"
	"github.com/K-Phoen/grabana/target/prometheus"
	"github.com/K-Phoen/grabana/timeseries"
	"gopkg.in/yaml.v2"
)

type RecordingRule struct {
	Name string `yaml:"record"`
	Expr string `yaml:"expr"`
}

type RecodingMap struct {
	data  map[[16]byte]RecordingRule
	debug bool
}

// NewRecordingMap if debug show queries instant of recording rule
func NewRecordingMap(debug bool) RecodingMap {
	return RecodingMap{
		data:  map[[16]byte]RecordingRule{},
		debug: debug,
	}
}

type PrometheusGroups struct {
	Groups []PrometheusGroup `yaml:"groups"`
}

type PrometheusGroup struct {
	Name  string          `yaml:"name"`
	Rules []RecordingRule `yaml:"rules"`
}

type K8sRule struct {
	ApiVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   K8sRuleMetadata  `yaml:"metadata"`
	Spec       PrometheusGroups `yaml:"spec"`
}

type K8sRuleMetadata struct {
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace"`
	Labels    map[string]string `yaml:"labels"`
}

func (m *RecodingMap) WritePrometheusK8sRulesYaml(name, filename string, metadata K8sRuleMetadata) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("Error creating K8sPrometheusRule Files: %w", err)
	}
	defer f.Close()
	data := K8sRule{
		ApiVersion: "monitoring.coreos.com/v1",
		Kind:       "PrometheusRule",
		Metadata:   metadata,
		Spec:       m.GetPrometheusGroup(name),
	}
	return yaml.NewEncoder(f).Encode(&data)
}

func (m *RecodingMap) AppendRule(recordName, query string) {
	m.record(recordName, query)
}

func (m *RecodingMap) WritePrometheusRulesYaml(name, filename string, checkOverPromtool bool) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("Error creating PrometheusRule Files: %w", err)
	}
	defer f.Close()
	groups := m.GetPrometheusGroup(name)
	err = yaml.NewEncoder(f).Encode(&groups)
	if err != nil {
		return err
	}
	if checkOverPromtool {
		cmd := exec.Command("promtool", "check", "rules", filename)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return nil
}

func (m *RecodingMap) GetPrometheusGroup(name string) PrometheusGroups {
	groups := PrometheusGroups{
		Groups: make([]PrometheusGroup, 0),
	}
	groups.Groups = append(groups.Groups, PrometheusGroup{
		Name:  name,
		Rules: make([]RecordingRule, 0),
	})
	for _, v := range m.data {
		groups.Groups[0].Rules = append(groups.Groups[0].Rules, v)
	}
	return groups
}

func (m *RecodingMap) escapeControlSymbols(expr string) string {
	re := regexp.MustCompile(`\s+`)
	result := re.ReplaceAllString(expr, " ")
	return strings.ReplaceAll(strings.ReplaceAll(result, "\n", " "), "\t", " ")
}

func (m *RecodingMap) record(name, expr string) [16]byte {
	escapedExpr := m.escapeControlSymbols(expr)
	hash := md5.Sum([]byte(escapedExpr))
	if _, cached := m.data[hash]; !cached {
		m.data[hash] = RecordingRule{
			Name: name,
			Expr: escapedExpr,
		}
	} else {
		log.Printf("Same expr found. Rule found under name: %s. will use this also for %s \n", m.data[hash].Name, name)
	}
	return hash
}

func (m *RecodingMap) Record(name string) prometheus.Option {
	return func(p *prometheus.Prometheus) {
		hash := m.record(name, p.Expr)
		p.Expr = m.data[hash].Name
	}
}

func ShouldShowQuery(show bool) prometheus.Option {
	if !show {
		return prometheus.Hide()
	} else {
		return func(target *prometheus.Prometheus) {}
	}
}

func (m *RecodingMap) WithTable(recordName, query string, options ...prometheus.Option) table.Option {
	return func(t *table.Table) error {
		completeRecordName := recordName
		err := table.WithPrometheusTarget(query, append([]prometheus.Option{
			m.Record(completeRecordName),
			ShouldShowQuery(!m.debug),
			prometheus.Ref(recordName),
		}, options...)...)(t)
		if err != nil {
			return err
		}
		err = table.WithPrometheusTarget(query, append([]prometheus.Option{
			ShouldShowQuery(m.debug),
			prometheus.Ref(fmt.Sprintf("%s_direct", recordName)),
		}, options...)...)(t)
		return err
	}
}

func (m *RecodingMap) WithSingleStat(recordName, query string, options ...prometheus.Option) singlestat.Option {
	return func(t *singlestat.SingleStat) error {
		completeRecordName := recordName
		err := singlestat.WithPrometheusTarget(query, append([]prometheus.Option{
			m.Record(completeRecordName),
			ShouldShowQuery(!m.debug),
			prometheus.Ref(recordName),
		}, options...)...)(t)
		if err != nil {
			return err
		}
		err = singlestat.WithPrometheusTarget(query, append([]prometheus.Option{
			ShouldShowQuery(m.debug),
			prometheus.Ref(fmt.Sprintf("%s_direct", recordName)),
		}, options...)...)(t)
		return err
	}
}

func (m *RecodingMap) WithHeatmap(recordName, query string, options ...prometheus.Option) heatmap.Option {
	return func(t *heatmap.Heatmap) error {
		completeRecordName := recordName
		err := heatmap.WithPrometheusTarget(query, append([]prometheus.Option{
			m.Record(completeRecordName),
			ShouldShowQuery(!m.debug),
			prometheus.Ref(recordName),
		}, options...)...)(t)
		if err != nil {
			return err
		}
		err = heatmap.WithPrometheusTarget(query, append([]prometheus.Option{
			ShouldShowQuery(m.debug),
			prometheus.Ref(fmt.Sprintf("%s_direct", recordName)),
		}, options...)...)(t)
		return err
	}
}

func (m *RecodingMap) WithStat(recordName, query string, options ...prometheus.Option) stat.Option {
	return func(t *stat.Stat) error {
		completeRecordName := recordName
		err := stat.WithPrometheusTarget(query, append([]prometheus.Option{
			m.Record(completeRecordName),
			ShouldShowQuery(!m.debug),
			prometheus.Ref(recordName),
		}, options...)...)(t)
		if err != nil {
			return err
		}
		err = stat.WithPrometheusTarget(query, append([]prometheus.Option{
			ShouldShowQuery(m.debug),
			prometheus.Ref(fmt.Sprintf("%s_direct", recordName)),
		}, options...)...)(t)
		return err
	}
}

func (m *RecodingMap) WithGraph(recordName, query string, options ...prometheus.Option) graph.Option {
	return func(t *graph.Graph) error {
		completeRecordName := recordName
		err := graph.WithPrometheusTarget(query, append([]prometheus.Option{
			m.Record(completeRecordName),
			ShouldShowQuery(!m.debug),
			prometheus.Ref(recordName),
		}, options...)...)(t)
		if err != nil {
			return err
		}
		err = graph.WithPrometheusTarget(query, append([]prometheus.Option{
			ShouldShowQuery(m.debug),
			prometheus.Ref(fmt.Sprintf("%s_direct", recordName)),
		}, options...)...)(t)
		return err
	}
}

func (m *RecodingMap) WithGauge(recordName, query string, options ...prometheus.Option) gauge.Option {
	return func(t *gauge.Gauge) error {
		completeRecordName := recordName
		err := gauge.WithPrometheusTarget(query, append([]prometheus.Option{
			m.Record(completeRecordName),
			ShouldShowQuery(!m.debug),
			prometheus.Ref(recordName),
		}, options...)...)(t)
		if err != nil {
			return err
		}
		err = gauge.WithPrometheusTarget(query, append([]prometheus.Option{
			ShouldShowQuery(m.debug),
			prometheus.Ref(fmt.Sprintf("%s_direct", recordName)),
		}, options...)...)(t)
		return err
	}
}

func (m *RecodingMap) WithTimeSeries(recordName, query string, options ...prometheus.Option) timeseries.Option {
	return func(t *timeseries.TimeSeries) error {
		completeRecordName := recordName
		err := timeseries.WithPrometheusTarget(query, append([]prometheus.Option{
			m.Record(completeRecordName),
			ShouldShowQuery(!m.debug),
			prometheus.Ref(recordName),
		}, options...)...)(t)
		if err != nil {
			return err
		}
		err = timeseries.WithPrometheusTarget(query, append([]prometheus.Option{
			ShouldShowQuery(m.debug),
			prometheus.Ref(fmt.Sprintf("%s_direct", recordName)),
		}, options...)...)(t)
		return err
	}
	// return []timeseries.Option{
	// 	timeseries.WithPrometheusTarget(query, append([]prometheus.Option{
	// 		m.Record(recordName),
	// 		ShouldShowQuery(!m.debug),
	// 	}, options...)...),
	// 	timeseries.WithPrometheusTarget(query, append([]prometheus.Option{
	// 		ShouldShowQuery(m.debug),
	// 	}, options...)...),
	// }
}

// func (m RecodingMap) Recoding(querySet string) func(t *timeseries.TimeSeries) error {
// 	return func(t *timeseries.TimeSeries) error {
// 		test := t.Builder.TimeseriesPanel.Targets
// 		for _, r := range test {

// 			if _, cached := m[hash]; !cached {
// 				metricName := r.MetricName
// 				name := fmt.Sprintf("%s:%s", querySet, metricName)
// 				m[hash] = RecordingRules{
// 					Name: name,
// 					Expr: r.Expr,
// 				}
// 			}
// 			r.Expr = m[hash].Expr
// 			t.
// 		}
// 		return nil
// 	}

// }
