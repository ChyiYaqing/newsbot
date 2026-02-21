package notify

import "context"

// Notifier defines a notification channel.
type Notifier interface {
	Send(ctx context.Context, title, body string) error
}
