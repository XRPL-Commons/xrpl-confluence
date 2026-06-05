package generator

import (
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	addresscodec "github.com/Peersyst/xrpl-go/address-codec"
)

// Ledger object IDs for the "reference" transaction types (CheckCash,
// PaymentChannelClaim, MPTokenIssuanceSet, …) are derived deterministically
// from the creating transaction's account + sequence — exactly as rippled and
// goXRPL compute them in their keylet packages. Deriving them here lets the
// generator build well-formed follow-up transactions that hit real on-ledger
// objects rather than random (always-failing) IDs.
//
// The values below mirror goXRPL/keylet/keylet.go: a ledger index is
// SHA-512Half( BigEndian(space) || field... ), i.e. the first 32 bytes of the
// SHA-512 digest. The 16-bit "space" prefix selects the object namespace.
const (
	spaceCheck      uint16 = 'C' // 0x0043
	spacePayChannel uint16 = 'x' // 0x0078
	spaceNFTokenOff uint16 = 'q' // 0x0071
	spacePermDomain uint16 = 'm' // 0x006D
)

// sha512Half returns the first 32 bytes of SHA-512 over the concatenated inputs.
func sha512Half(parts ...[]byte) [32]byte {
	h := sha512.New()
	for _, p := range parts {
		h.Write(p)
	}
	full := h.Sum(nil)
	var out [32]byte
	copy(out[:], full[:32])
	return out
}

// indexHash computes a ledger index: SHA-512Half( BE16(space) || fields... ).
func indexHash(space uint16, fields ...[]byte) [32]byte {
	var sp [2]byte
	binary.BigEndian.PutUint16(sp[:], space)
	parts := make([][]byte, 0, len(fields)+1)
	parts = append(parts, sp[:])
	parts = append(parts, fields...)
	return sha512Half(parts...)
}

// accountIDBytes decodes a classic address ("r…") to its 20-byte AccountID.
func accountIDBytes(address string) ([]byte, error) {
	_, id, err := addresscodec.DecodeClassicAddressToAccountID(address)
	if err != nil {
		return nil, fmt.Errorf("decode %q: %w", address, err)
	}
	return id, nil
}

func seqBytes(seq uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], seq)
	return b[:]
}

// hexUpper renders bytes as the uppercase hex XRPL uses for Hash256/blob fields.
func hexUpper(b []byte) string { return strings.ToUpper(hex.EncodeToString(b)) }

// checkID derives the Check ledger object ID (CheckID field of CheckCash /
// CheckCancel) from the CheckCreate account and the sequence it was assigned.
func checkID(owner string, seq uint32) (string, error) {
	id, err := accountIDBytes(owner)
	if err != nil {
		return "", err
	}
	h := indexHash(spaceCheck, id, seqBytes(seq))
	return hexUpper(h[:]), nil
}

// payChannelID derives the PayChannel ledger object ID (Channel field of
// PaymentChannelFund / PaymentChannelClaim) from the create's source,
// destination and sequence.
func payChannelID(source, destination string, seq uint32) (string, error) {
	src, err := accountIDBytes(source)
	if err != nil {
		return "", err
	}
	dst, err := accountIDBytes(destination)
	if err != nil {
		return "", err
	}
	h := indexHash(spacePayChannel, src, dst, seqBytes(seq))
	return hexUpper(h[:]), nil
}

// nftokenOfferID derives the NFTokenOffer ledger object ID (used by
// NFTokenAcceptOffer / NFTokenCancelOffer) from the offer creator and sequence.
func nftokenOfferID(owner string, seq uint32) (string, error) {
	id, err := accountIDBytes(owner)
	if err != nil {
		return "", err
	}
	h := indexHash(spaceNFTokenOff, id, seqBytes(seq))
	return hexUpper(h[:]), nil
}

// permissionedDomainID derives the PermissionedDomain ledger object ID
// (DomainID field of PermissionedDomainDelete) from the owner and sequence.
func permissionedDomainID(owner string, seq uint32) (string, error) {
	id, err := accountIDBytes(owner)
	if err != nil {
		return "", err
	}
	h := indexHash(spacePermDomain, id, seqBytes(seq))
	return hexUpper(h[:]), nil
}

// mptIssuanceID derives the 192-bit MPTokenIssuanceID — a direct concatenation
// of BigEndian(sequence) || issuerAccountID (NOT a hash), matching
// rippled's makeMptID / goXRPL's MakeMPTID.
func mptIssuanceID(issuer string, seq uint32) (string, error) {
	id, err := accountIDBytes(issuer)
	if err != nil {
		return "", err
	}
	out := make([]byte, 0, 24)
	out = append(out, seqBytes(seq)...)
	out = append(out, id...)
	return hexUpper(out), nil
}
