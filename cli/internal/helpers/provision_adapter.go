// Where: cli/internal/helpers/provision_adapter.go
// What: Provisioner alias to ports definitions.
// Why: Allow CLI to continue referencing app.Provisioner while workflows import ports.
package helpers

import "github.com/poruru/edge-serverless-box/cli/internal/ports"

type Provisioner = ports.Provisioner
