package coderd

import (
	"net/http"
	"reflect"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

// FeaturesService is the interface for interacting with enterprise features.
type FeaturesService interface {
	EntitlementsAPI(w http.ResponseWriter, r *http.Request)

	// Get returns the implementations for feature interfaces. Parameter `s` must be a pointer to a
	// struct type containing feature interfaces as fields.  The FeatureService sets all fields to
	// the correct implementations depending on whether the features are turned on.
	Get(s any) error
}

type featuresService struct{}

func (featuresService) EntitlementsAPI(rw http.ResponseWriter, _ *http.Request) {
	features := make(map[string]codersdk.Feature)
	for _, f := range codersdk.FeatureNames {
		features[f] = codersdk.Feature{
			Entitlement: codersdk.EntitlementNotEntitled,
			Enabled:     false,
		}
	}
	httpapi.Write(rw, http.StatusOK, codersdk.Entitlements{
		Features:   features,
		Warnings:   []string{},
		HasLicense: false,
	})
}

// Get returns the implementations for feature interfaces. Parameter `s` must be a pointer to a
// struct type containing feature interfaces as fields.  The AGPL featureService always returns the
// "disabled" version of the feature interface because it doesn't include any enterprise features
// by definition.
func (featuresService) Get(ps any) error {
	if reflect.TypeOf(ps).Kind() != reflect.Pointer {
		return xerrors.New("input must be pointer to struct")
	}
	vs := reflect.ValueOf(ps).Elem()
	if vs.Kind() != reflect.Struct {
		return xerrors.New("input must be pointer to struct")
	}
	for i := 0; i < vs.NumField(); i++ {
		vf := vs.Field(i)
		tf := vf.Type()
		if tf.Kind() != reflect.Interface {
			return xerrors.Errorf("fields of input struct must be interfaces: %s", tf.String())
		}
		err := setImplementation(vf, tf)
		if err != nil {
			return err
		}
	}
	return nil
}

// setImplementation finds the correct implementation for the field's type, and sets it on the
// struct.  It returns an error if unsuccessful
func setImplementation(vf reflect.Value, tf reflect.Type) error {
	// when we get more than a few features it might make sense to have a data structure for finding
	// the correct implementation that's faster than just a linear search, but for now just spin
	// through the implementations we have.
	vd := reflect.ValueOf(DisabledImplementations)
	for j := 0; j < vd.NumField(); j++ {
		vdf := vd.Field(j)
		if vdf.Type() == tf {
			vf.Set(vdf)
			return nil
		}
	}
	return xerrors.Errorf("unable to find implementation of interface %s", tf.String())
}

// FeatureInterfaces contains a field for each interface controlled by an enterprise feature.
type FeatureInterfaces struct {
	Auditor audit.Auditor
}

// DisabledImplementations includes all the implementations of turned-off features.  There are no
// turned-on implementations in AGPL code.
var DisabledImplementations = FeatureInterfaces{
	Auditor: audit.NewNop(),
}
