package notify

import (
	"context"

	"github.com/slack-go/slack"
)

type RealSlack struct {
	api *slack.Client
}

func NewRealSlack(token string) *RealSlack {
	return &RealSlack{api: slack.New(token)}
}

func (r *RealSlack) DM(ctx context.Context, userID, text string) error {
	channel, _, _, err := r.api.OpenConversationContext(ctx,
		&slack.OpenConversationParameters{Users: []string{userID}})
	if err != nil {
		return err
	}
	_, _, err = r.api.PostMessageContext(ctx, channel.ID, slack.MsgOptionText(text, false))
	return err
}
