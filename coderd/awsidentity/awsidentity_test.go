package awsidentity_test

import (
	"crypto/rsa"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/awsidentity"
)

const (
	signature = `M7rX9w1s5zK1V7hK0dsE4hTDXHHaaDuKQ9iIz/W8ZNaA2lJ/usz5YuX+ORt3luJwswl/+B7cYOkJ
bXRMx/pEQ6vT+niLGZDC9ZZ1h9Ox4h4e4m4IisQSCUrVIzyLj+MB27/Wyy0NhXcpoZVjNEmioxF2
HNpOR4aCwUxxOm81y98=`
	document = `{
  "accountId" : "628783029487",
  "architecture" : "x86_64",
  "availabilityZone" : "us-east-1b",
  "billingProducts" : null,
  "devpayProductCodes" : null,
  "marketplaceProductCodes" : null,
  "imageId" : "ami-0c02fb55956c7d316",
  "instanceId" : "i-076e9b91f7c420782",
  "instanceType" : "t2.micro",
  "kernelId" : null,
  "pendingTime" : "2022-03-25T20:07:16Z",
  "privateIp" : "172.31.84.238",
  "ramdiskId" : null,
  "region" : "us-east-1",
  "version" : "2017-09-30"
}`
)

func TestValidate(t *testing.T) {
	t.Parallel()
	t.Run("FailEmpty", func(t *testing.T) {
		t.Parallel()
		_, err := awsidentity.Validate("", "", nil)
		require.Error(t, err)
	})
	t.Run("FailBad", func(t *testing.T) {
		t.Parallel()
		_, err := awsidentity.Validate(signature, "{}", nil)
		require.ErrorIs(t, err, rsa.ErrVerification)
	})
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		identity, err := awsidentity.Validate(signature, document, nil)
		require.NoError(t, err)
		require.Equal(t, awsidentity.Other, identity.Region)
	})
}
