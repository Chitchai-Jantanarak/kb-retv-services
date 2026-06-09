package dto

type InboundMessageRequest struct {
	Channel           string            `json:"channel" validate:"required,oneof=line email web voice api"`
	ExternalMessageID string            `json:"external_message_id" validate:"required"`
	CustomerID        string            `json:"customer_id,omitempty"`
	SiteID            string            `json:"site_id,omitempty"`
	ConversationID    string            `json:"conversation_id,omitempty"`
	Subject           string            `json:"subject,omitempty"`
	Body              string            `json:"body,omitempty"`
	Attachments       []AttachmentRef   `json:"attachments,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

func (r *InboundMessageRequest) Validate() error {
	r.Normalize()
	if err := validateAttachmentRefs(r.Attachments); err != nil {
		return err
	}
	if err := requireBodyOrAttachment(r.Body, r.Attachments); err != nil {
		return err
	}
	return validateStruct(r)
}

func (r *InboundMessageRequest) Normalize() {
	r.Channel = normalizeText(r.Channel)
	r.ExternalMessageID = normalizeText(r.ExternalMessageID)
	r.CustomerID = normalizeText(r.CustomerID)
	r.SiteID = normalizeText(r.SiteID)
	r.ConversationID = normalizeText(r.ConversationID)
	r.Subject = normalizeText(r.Subject)
	r.Body = normalizeText(r.Body)
}
