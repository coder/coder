package cliutil

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

// NewLicenseFormatter returns a new license formatter.
// The formatter will return a table and JSON output.
func NewLicenseFormatter() *cliui.OutputFormatter {
	type tableLicense struct {
		ID         int32     `table:"id,default_sort"`
		UUID       uuid.UUID `table:"uuid" format:"uuid"`
		UploadedAt time.Time `table:"uploaded at" format:"date-time"`
		// Features is the formatted string for the license claims.
		// Used for the table view.
		Features  string    `table:"features"`
		ExpiresAt time.Time `table:"expires at" format:"date-time"`
		Trial     bool      `table:"trial"`
	}

	return cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat([]tableLicense{}, []string{"ID", "UUID", "Expires At", "Uploaded At", "Features"}),
			func(data any) (any, error) {
				list, ok := data.([]codersdk.License)
				if !ok {
					return nil, xerrors.Errorf("invalid data type %T", data)
				}
				out := make([]tableLicense, 0, len(list))
				for _, lic := range list {
					var formattedFeatures string
					features, err := lic.FeaturesClaims()
					if err != nil {
						formattedFeatures = xerrors.Errorf("invalid license: %w", err).Error()
					} else {
						var strs []string
						if lic.AllFeaturesClaim() {
							// If all features are enabled, just include that
							strs = append(strs, "all features")
						} else {
							for k, v := range features {
								if v > 0 {
									// Only include claims > 0
									strs = append(strs, fmt.Sprintf("%s=%v", k, v))
								}
							}
						}
						formattedFeatures = strings.Join(strs, ", ")
					}
					// If this returns an error, a zero time is returned.
					exp, _ := lic.ExpiresAt()

					out = append(out, tableLicense{
						ID:         lic.ID,
						UUID:       lic.UUID,
						UploadedAt: lic.UploadedAt,
						Features:   formattedFeatures,
						ExpiresAt:  exp,
						Trial:      lic.Trial(),
					})
				}
				return out, nil
			}),
		cliui.ChangeFormatterData(cliui.JSONFormat(), func(data any) (any, error) {
			list, ok := data.([]codersdk.License)
			if !ok {
				return nil, xerrors.Errorf("invalid data type %T", data)
			}
			for i := range list {
				humanExp, err := list[i].ExpiresAt()
				if err == nil {
					list[i].Claims[codersdk.LicenseExpiryClaim+"_human"] = humanExp.Format(time.RFC3339)
				}
			}

			return list, nil
		}),
	)
}
