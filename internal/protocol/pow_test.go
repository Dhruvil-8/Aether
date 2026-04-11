package protocol

import "testing"

func TestMineAndVerify(t *testing.T) {
	msg := &Message{
		V: 1,
		T: 1710000000,
		C: "hello",
	}
	msg.H = msg.ComputeHash()
	nonce, err := MinePoW(msg.H, 12)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPoW(msg.H, nonce, 12)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected valid pow")
	}
}
