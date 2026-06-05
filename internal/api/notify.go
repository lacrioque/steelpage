package api

import (
	"fmt"
	"html"
	"log"
	"net/url"
	"strings"

	"github.com/markusfluer/steelpage/internal/comments"
	"github.com/markusfluer/steelpage/internal/mailer"
	"github.com/markusfluer/steelpage/internal/notifications"
	"github.com/markusfluer/steelpage/internal/users"
)

// notifyForComment fans out notifications (and opt-in emails) for a freshly
// created or edited comment. Called in a goroutine after the HTTP response is
// written — failures are logged, never surfaced to the commenter.
//
//   - Mentions: every user whose display name appears as "@Name" in the body.
//     On edit, only names NOT already mentioned in prevBody re-notify, so
//     fixing a typo doesn't re-ping everyone.
//   - Replies (create only): every distinct participant of the thread the
//     comment replies to — root author plus earlier repliers — except anyone
//     already covered by a mention notification for this same comment.
func (a *API) notifyForComment(actor *users.User, c *comments.Comment, prevBody string, isUpdate bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("notifications: panic while notifying for comment id=%d: %v", c.ID, r)
		}
	}()

	mentionable, err := a.Users.Mentionable()
	if err != nil {
		log.Printf("notifications: list mentionable users: %v", err)
		return
	}
	names := make([]string, len(mentionable))
	byName := make(map[string][]int64, len(mentionable))
	for i, m := range mentionable {
		names[i] = m.DisplayName
		byName[m.DisplayName] = append(byName[m.DisplayName], m.ID)
	}

	matched := notifications.ParseMentions(c.Body, names)
	if isUpdate {
		prev := make(map[string]bool)
		for _, n := range notifications.ParseMentions(prevBody, names) {
			prev[n] = true
		}
		fresh := matched[:0]
		for _, n := range matched {
			if !prev[n] {
				fresh = append(fresh, n)
			}
		}
		matched = fresh
	}

	notified := map[int64]bool{actor.ID: true} // never notify the actor
	for _, name := range matched {
		for _, uid := range byName[name] {
			if notified[uid] {
				continue
			}
			notified[uid] = true
			if err := a.Notifications.Create(uid, actor.ID, c.ID, notifications.KindMention, c.Path); err != nil {
				log.Printf("notifications: create mention for user_id=%d comment_id=%d: %v", uid, c.ID, err)
				continue
			}
			a.maybeEmail(uid, notifications.KindMention, actor, c)
		}
	}

	if isUpdate || c.ReplyTo == nil {
		return
	}
	participants, err := a.Comments.ThreadParticipantIDs(*c.ReplyTo)
	if err != nil {
		log.Printf("notifications: thread participants for comment_id=%d: %v", *c.ReplyTo, err)
		return
	}
	for _, uid := range participants {
		if notified[uid] {
			continue // actor, or already pinged via mention
		}
		notified[uid] = true
		if err := a.Notifications.Create(uid, actor.ID, c.ID, notifications.KindReply, c.Path); err != nil {
			log.Printf("notifications: create reply for user_id=%d comment_id=%d: %v", uid, c.ID, err)
			continue
		}
		a.maybeEmail(uid, notifications.KindReply, actor, c)
	}
}

// maybeEmail sends the notification email when every gate passes: SMTP is
// configured, the recipient has a verified email, and their per-kind
// preference is on. Best effort — failures are logged only.
func (a *API) maybeEmail(recipientID int64, kind notifications.Kind, actor *users.User, c *comments.Comment) {
	if !a.Mailer.Enabled() {
		return
	}
	recipient, err := a.Users.GetByID(recipientID)
	if err != nil {
		log.Printf("notifications: load recipient user_id=%d: %v", recipientID, err)
		return
	}
	if recipient.Email == nil || recipient.EmailVerifiedAt == nil {
		return
	}
	switch kind {
	case notifications.KindMention:
		if !recipient.EmailOnMention {
			return
		}
	case notifications.KindReply:
		if !recipient.EmailOnResponse {
			return
		}
	default:
		return
	}

	link := a.docURL(c.Path)
	var subject, textBody, htmlBody string
	switch kind {
	case notifications.KindMention:
		subject = fmt.Sprintf("%s mentioned you on %s", actor.DisplayName, c.Path)
		textBody = fmt.Sprintf(`Hi %s,

%s mentioned you in a comment on %s:

%s

Open the page: %s

— Steelpage
`, recipient.DisplayName, actor.DisplayName, c.Path, c.Body, link)
		htmlBody = fmt.Sprintf(`<p>Hi %s,</p>
<p><strong>%s</strong> mentioned you in a comment on <strong>%s</strong>:</p>
<blockquote>%s</blockquote>
<p><a href="%s">Open the page</a></p>
<p>— Steelpage</p>`,
			html.EscapeString(recipient.DisplayName), html.EscapeString(actor.DisplayName),
			html.EscapeString(c.Path), html.EscapeString(c.Body), link)
	case notifications.KindReply:
		subject = fmt.Sprintf("%s replied to your comment on %s", actor.DisplayName, c.Path)
		textBody = fmt.Sprintf(`Hi %s,

%s replied in a comment thread you took part in on %s:

%s

Open the page: %s

— Steelpage
`, recipient.DisplayName, actor.DisplayName, c.Path, c.Body, link)
		htmlBody = fmt.Sprintf(`<p>Hi %s,</p>
<p><strong>%s</strong> replied in a comment thread you took part in on <strong>%s</strong>:</p>
<blockquote>%s</blockquote>
<p><a href="%s">Open the page</a></p>
<p>— Steelpage</p>`,
			html.EscapeString(recipient.DisplayName), html.EscapeString(actor.DisplayName),
			html.EscapeString(c.Path), html.EscapeString(c.Body), link)
	}

	if err := a.Mailer.Send(mailer.Message{
		To:      []string{*recipient.Email},
		Subject: subject,
		Text:    textBody,
		HTML:    htmlBody,
	}); err != nil {
		log.Printf("notifications: %s email FAILED for user_id=%d comment_id=%d: %v", kind, recipient.ID, c.ID, err)
		return
	}
	log.Printf("notifications: %s email queued for user_id=%d comment_id=%d", kind, recipient.ID, c.ID)
}

// docURL builds the absolute link to a document, mirroring auth's publicURL:
// live server.base_url (admin-editable) with a bind-address fallback.
func (a *API) docURL(path string) string {
	live := a.cfg()
	base := live.Server.BaseURL
	if base == "" {
		base = "http://" + live.Server.Bind
	}
	segs := strings.Split(path, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.TrimRight(base, "/") + "/docs/" + strings.Join(segs, "/")
}
