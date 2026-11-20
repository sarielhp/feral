package outlook

import (
	"testing"

	"github.com/sarielhp/feral/backend/backendtest"
)

func TestOutlookClient_MessageAccessorContextCancellation(t *testing.T) {
	backendtest.AssertAccessorHonorsCancellation(t, &client{})
}
