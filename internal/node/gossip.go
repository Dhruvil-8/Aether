package node

import (
	"errors"

	"aether/internal/protocol"
)

var ErrInvalidPoW = errors.New("invalid proof-of-work")

type Gossip struct{}

func NewGossip() *Gossip {
	return &Gossip{}
}

func (g *Gossip) Validate(msg *protocol.Message) error {
	if err := msg.ValidateContent(); err != nil {
		return err
	}
	if err := msg.VerifyHash(); err != nil {
		return err
	}
	ok, err := protocol.VerifyPoW(msg.H, msg.P, protocol.Difficulty)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidPoW
	}
	return nil
}
