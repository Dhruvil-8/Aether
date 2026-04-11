package protocol

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/bits"
)

const Difficulty = 20

var ErrMiningExhausted = errors.New("unable to find valid nonce")

func VerifyPoW(hashHex string, nonce uint32, difficulty int) (bool, error) {
	hashBytes, err := DecodeHashHex(hashHex)
	if err != nil {
		return false, err
	}
	powInput := make([]byte, len(hashBytes)+4)
	copy(powInput, hashBytes)
	binary.BigEndian.PutUint32(powInput[len(hashBytes):], nonce)
	sum := sha256.Sum256(powInput)
	return leadingZeroBits(sum[:]) >= difficulty, nil
}

func MinePoW(hashHex string, difficulty int) (uint32, error) {
	for nonce := uint32(0); ; nonce++ {
		ok, err := VerifyPoW(hashHex, nonce, difficulty)
		if err != nil {
			return 0, err
		}
		if ok {
			return nonce, nil
		}
		if nonce == ^uint32(0) {
			return 0, ErrMiningExhausted
		}
	}
}

func leadingZeroBits(input []byte) int {
	total := 0
	for _, b := range input {
		if b == 0 {
			total += 8
			continue
		}
		total += bits.LeadingZeros8(uint8(b))
		break
	}
	return total
}
