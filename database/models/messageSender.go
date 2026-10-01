package models

import "time"

type MessageSenderProvider struct {
	Name     string `json:"name" gorm:"primaryKey;unique;not null"`
	Addition string `json:"addition" gorm:"type:longtext" default:"{}"`
}

type EventMessage struct {
	Event   any       `json:"event"`
	Clients []Client  `json:"clients"`
	Time    time.Time `json:"time"`
	Message any       `json:"message"`
	Emoji   any       `json:"emoji"`
	Title   string    `json:"title,omitempty"`
	// Template is optional. When set, the sender renders this event with the
	// supplied template before dispatching it as a text notification.
	Template string `json:"template,omitempty"`
	Task string `json:"task,omitempty"`
	LossRate string `json:"loss_rate,omitempty"`
	Window string `json:"window,omitempty"`
}
