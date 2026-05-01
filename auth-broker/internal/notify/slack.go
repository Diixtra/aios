package notify

import (
	"context"
	"fmt"
)

type SlackClient interface {
	DM(ctx context.Context, userID, text string) error
}

type Notifier struct {
	client    SlackClient
	userID    string
	brokerURL string
}

func NewNotifier(c SlackClient, userID, brokerURL string) *Notifier {
	return &Notifier{client: c, userID: userID, brokerURL: brokerURL}
}

const recipeTemplate = `AIOS auth needs attention (reason: %s).

On a laptop with pi installed, run ` + "`pi /login`" + ` (complete OAuth in browser, then exit pi), then:

    just bootstrap-auth   # uploads ~/.pi/agent/auth.json to %s

The agent queue is paused until the upload validates.`

func (n *Notifier) BootstrapRecipe(ctx context.Context, reason string) error {
	return n.client.DM(ctx, n.userID, fmt.Sprintf(recipeTemplate, reason, n.brokerURL))
}

func (n *Notifier) Warning(ctx context.Context, ageDays int) error {
	return n.client.DM(ctx, n.userID,
		fmt.Sprintf("AIOS auth bundle is %dd old — refresh at your convenience: re-run `pi /login` on laptop and `just bootstrap-auth` upload.", ageDays))
}

func (n *Notifier) Recovered(ctx context.Context) error {
	return n.client.DM(ctx, n.userID, "AIOS reauthenticated, queue draining.")
}
