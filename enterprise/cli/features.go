package cli
import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)
func (r *RootCmd) features() *serpent.Command {
	cmd := &serpent.Command{
		Short:   "List Enterprise features",
		Use:     "features",
		Aliases: []string{"feature"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.featuresList(),
		},
	}
	return cmd
}
func (r *RootCmd) featuresList() *serpent.Command {
	var (
		featureColumns = []string{"name", "entitlement", "enabled", "limit", "actual"}
		columns        []string
		outputFormat   string
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Middleware: serpent.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			entitlements, err := client.Entitlements(inv.Context())
			var apiError *codersdk.Error
			if errors.As(err, &apiError) && apiError.StatusCode() == http.StatusNotFound {
				return errors.New("You are on the AGPL licensed version of Coder that does not have Enterprise functionality!")
			}
			if err != nil {
				return err
			}
			// This uses custom formatting as the JSON output outputs an object
			// as opposed to a list from the table output.
			out := ""
			switch outputFormat {
			case "table", "":
				out, err = displayFeatures(columns, entitlements.Features)
				if err != nil {
					return fmt.Errorf("render table: %w", err)
				}
			case "json":
				buf := new(bytes.Buffer)
				enc := json.NewEncoder(buf)
				enc.SetIndent("", "  ")
				err = enc.Encode(entitlements)
				if err != nil {
					return fmt.Errorf("marshal features to JSON: %w", err)
				}
				out = buf.String()
			default:
				return fmt.Errorf(`unknown output format %q, only "table" and "json" are supported`, outputFormat)
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:          "column",
			FlagShorthand: "c",
			Description:   "Specify columns to filter in the table.",
			Default:       strings.Join(featureColumns, ","),
			Value:         serpent.EnumArrayOf(&columns, featureColumns...),
		},
		{
			Flag:          "output",
			FlagShorthand: "o",
			Description:   "Output format.",
			Default:       "table",
			Value:         serpent.EnumOf(&outputFormat, "table", "json"),
		},
	}
	return cmd
}
type featureRow struct {
	Name        codersdk.FeatureName `table:"name,default_sort"`
	Entitlement string               `table:"entitlement"`
	Enabled     bool                 `table:"enabled"`
	Limit       *int64               `table:"limit"`
	Actual      *int64               `table:"actual"`
}
// displayFeatures will return a table displaying all features passed in.
// filterColumns must be a subset of the feature fields and will determine which
// columns to display
func displayFeatures(filterColumns []string, features map[codersdk.FeatureName]codersdk.Feature) (string, error) {
	rows := make([]featureRow, 0, len(features))
	for name, feat := range features {
		rows = append(rows, featureRow{
			Name:        name,
			Entitlement: string(feat.Entitlement),
			Enabled:     feat.Enabled,
			Limit:       feat.Limit,
			Actual:      feat.Actual,
		})
	}
	return cliui.DisplayTable(rows, "name", filterColumns)
}
