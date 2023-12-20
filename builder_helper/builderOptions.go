package builderhelper

import (
	"github.com/K-Phoen/grabana/stat"
	"github.com/K-Phoen/grabana/timeseries/fields"
	"github.com/K-Phoen/grabana/variable/text"
	"github.com/K-Phoen/sdk"
)

type ContinuousColorSchemeType string

const (
	RYG ContinuousColorSchemeType = "Red-Yellow-Green"
)

func ContinuousColorScheme(color ContinuousColorSchemeType) fields.OverrideOption {
	return func(field *sdk.FieldConfigOverride) {
		field.Properties = append(field.Properties,
			sdk.FieldConfigOverrideProperty{
				ID: "color",
				Value: map[string]string{
					"mode":       "continuous-RdYlGr",
					"fixedColor": string(color),
				},
			})
	}
}

func StatFieldOverride(m fields.Matcher, opts ...fields.OverrideOption) stat.Option {
	return func(stats *stat.Stat) error {
		override := sdk.FieldConfigOverride{}

		m(&override)

		for _, opt := range opts {
			opt(&override)
		}
		stats.Builder.StatPanel.FieldConfig.Overrides = append(stats.Builder.StatPanel.FieldConfig.Overrides, override)
		return nil
	}
}

func VariableAsTextDefault(textValue string) func(*text.Text) {
	return func(t *text.Text) {
		t.Builder.Current = sdk.Current{
			Text:  &sdk.StringSliceString{Value: []string{textValue}},
			Value: textValue,
		}
	}

}
