package cli

import (
	"os"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli/podidentityoperator"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// podIdentityOperator runs in a Kubernetes cluster and tells coderd how to
// recognize the cluster's pod tokens.
func (r *RootCmd) podIdentityOperator() *serpent.Command {
	var (
		operatorKey string
		name        string
		namespaces  []string
		interval    time.Duration
		kubeURL     string
		kubeToken   string
		kubeCA      string
	)
	cmd := &serpent.Command{
		Use:   "pod-identity-operator",
		Short: "Register a Kubernetes cluster with Coder, so that workspace daemons can authenticate as the pod they run in",
		Long: "Run this in the cluster. It reads the issuer and the public signing keys of the cluster's service account " +
			"tokens and registers them with Coder, again and again. Coder then recognizes the tokens of the cluster's pods " +
			"without a way to the cluster's API and without a credential for it.",
		Middleware: serpent.RequireNArgs(0),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			if r.clientURL == nil || r.clientURL.String() == "" {
				return xerrors.New("say where Coder is with --url or CODER_URL")
			}
			logger := slog.Make(sloghuman.Sink(inv.Stderr))
			if name == "" {
				name, _ = os.Hostname()
			}
			operator, err := podidentityoperator.New(podidentityoperator.Options{
				Logger:        logger,
				Coder:         codersdk.New(r.clientURL),
				OperatorKey:   operatorKey,
				Name:          name,
				Namespaces:    namespaces,
				Interval:      interval,
				KubeURL:       kubeURL,
				KubeTokenFile: kubeToken,
				KubeCAFile:    kubeCA,
			})
			if err != nil {
				return err
			}
			err = operator.Run(ctx)
			if xerrors.Is(err, ctx.Err()) {
				return nil
			}
			return err
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:        "operator-key",
			Env:         "CODER_POD_IDENTITY_OPERATOR_KEY",
			Description: "The key Coder accepts cluster registrations with. It is good for nothing else.",
			Value:       serpent.StringOf(&operatorKey),
		},
		{
			Flag:        "name",
			Env:         "CODER_POD_IDENTITY_CLUSTER_NAME",
			Description: "A name for the cluster. It defaults to the host's name.",
			Value:       serpent.StringOf(&name),
		},
		{
			Flag:        "namespace",
			Env:         "CODER_POD_IDENTITY_NAMESPACES",
			Description: "A namespace whose pods may be workspace daemons. Repeat it for several. It defaults to the operator's own.",
			Value:       serpent.StringArrayOf(&namespaces),
		},
		{
			Flag:        "interval",
			Env:         "CODER_POD_IDENTITY_INTERVAL",
			Description: "How often the registration is renewed. Coder stops believing a cluster that does not renew for two minutes.",
			Default:     "30s",
			Value:       serpent.DurationOf(&interval),
		},
		{
			Flag:        "kube-url",
			Env:         "CODER_POD_IDENTITY_KUBE_URL",
			Description: "The cluster's API. It defaults to the cluster this runs in.",
			Value:       serpent.StringOf(&kubeURL),
		},
		{
			Flag:        "kube-token-file",
			Env:         "CODER_POD_IDENTITY_KUBE_TOKEN_FILE",
			Description: "A service account token for the cluster's API. It defaults to this pod's.",
			Value:       serpent.StringOf(&kubeToken),
		},
		{
			Flag:        "kube-ca-file",
			Env:         "CODER_POD_IDENTITY_KUBE_CA_FILE",
			Description: "The certificate authority of the cluster's API. It defaults to this pod's.",
			Value:       serpent.StringOf(&kubeCA),
		},
	}
	return cmd
}
