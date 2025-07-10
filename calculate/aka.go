package calculate

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/free5gc/milenage"
	"github.com/free5gc/util/ueauth"
)

type UEAuth struct {
	K           []byte
	OPc         []byte
	RAND        []byte
	AUTN        []byte
	ServingName string

	// 결과값들
	RES      []byte
	CK       []byte
	IK       []byte
	AK       []byte
	KAUSF    []byte
	ResStar  []byte // XRES* - free5gc uses this name
	HResStar []byte // HXRES* - free5gc uses this name
}

// CalculateResStar - Calculate free5gc resStar vector (XRES*)
func (u *UEAuth) CalculateResStar() error {
	fmt.Printf("🔐 ========== FREE5GC RESSTAR CALCULATION ==========\n")

	// Validate input parameters
	if len(u.K) != 16 || len(u.OPc) != 16 || len(u.RAND) != 16 || len(u.AUTN) != 16 {
		return errors.New("invalid input parameter lengths")
	}

	var opcArray, kArray, randArray [16]uint8
	copy(opcArray[:], u.OPc)
	copy(kArray[:], u.K)
	copy(randArray[:], u.RAND)

	// Extract SQN and AMF from AUTN
	sqnXorAK := u.AUTN[0:6]
	amfBytes := u.AUTN[6:8]
	// amf := binary.BigEndian.Uint16(amfBytes)

	// Compute Milenage functions
	var res [8]uint8
	var ck [16]uint8
	var ik [16]uint8
	var ak [6]uint8

	err := milenage.F2345(opcArray[:], kArray[:], randArray[:], res[:], ck[:], ik[:], ak[:], nil)
	if err != nil {
		return fmt.Errorf("milenage F2345 failed: %w", err)
	}

	// Convert results to []byte
	u.RES = make([]byte, 8)
	u.CK = make([]byte, 16)
	u.IK = make([]byte, 16)
	u.AK = make([]byte, 6)

	copy(u.RES, res[:])
	copy(u.CK, ck[:])
	copy(u.IK, ik[:])
	copy(u.AK, ak[:])

	sqnBytes := make([]byte, 6)
	for i := 0; i < 6; i++ {
		sqnBytes[i] = sqnXorAK[i] ^ u.AK[i]
	}

	var macA [8]uint8
	var macS [8]uint8
	err = milenage.F1(opcArray[:], kArray[:], randArray[:], sqnBytes, amfBytes, macA[:], macS[:])
	if err != nil {
		return fmt.Errorf("milenage F1 failed: %w", err)
	}

	receivedMAC := u.AUTN[8:16]
	if !bytes.Equal(macA[:], receivedMAC) {
		return fmt.Errorf("MAC verification failed")
	}

	u.KAUSF, err = u.deriveKAUSFFree5GC(sqnBytes)
	if err != nil {
		return fmt.Errorf("KAUSF derivation failed: %w", err)
	}

	u.ResStar, err = u.deriveResStarFree5GC()
	if err != nil {
		return fmt.Errorf("RES* derivation failed: %w", err)
	}

	u.HResStar = u.deriveHResStar()

	fmt.Printf("\n🔐 ========== CALCULATION COMPLETE ==========\n")
	return nil
}

// deriveKAUSFFree5GC - Derive KAUSF using free5gc's GetKDFValue
func (u *UEAuth) deriveKAUSFFree5GC(sqn []byte) ([]byte, error) {
	// Key = CK || IK
	key := append(u.CK, u.IK...)

	// P0 = Serving Network Name
	p0 := []byte(u.ServingName)

	// P1 = SQN ⊕ AK
	sqnXorAK := make([]byte, 6)
	for i := 0; i < 6; i++ {
		sqnXorAK[i] = sqn[i] ^ u.AK[i]
	}

	// Use free5gc's GetKDFValue function
	// FC = "6A" for KAUSF derivation
	kausf, err := ueauth.GetKDFValue(key, ueauth.FC_FOR_KAUSF_DERIVATION, p0, sqnXorAK)
	if err != nil {
		return nil, fmt.Errorf("GetKDFValue for KAUSF failed: %w", err)
	}

	// KAUSF is 256 bits (32 bytes)
	if len(kausf) < 32 {
		return nil, fmt.Errorf("KAUSF length is less than 32 bytes: %d", len(kausf))
	}

	return kausf[:32], nil
}

func (u *UEAuth) deriveResStarFree5GC() ([]byte, error) {
	// 1) 올바른 키: CK || IK
	key := append(u.CK, u.IK...)

	// 2) 파라미터 준비
	p0 := []byte(u.ServingName) // P0 = ServingNetworkName
	p1 := u.RAND                // P1 = RAND
	p2 := u.RES                 // P2 = RES
	l0 := ueauth.KDFLen(p0)     // L0
	l1 := ueauth.KDFLen(p1)     // L1
	l2 := ueauth.KDFLen(p2)     // L2

	// 3) KDF 호출 (길이 필드 포함)
	full, err := ueauth.GetKDFValue(
		key,
		ueauth.FC_FOR_RES_STAR_XRES_STAR_DERIVATION,
		p0, l0,
		p1, l1,
		p2, l2,
	)
	if err != nil {
		return nil, fmt.Errorf("GetKDFValue for RES* failed: %w", err)
	}

	// 4) 마지막 16바이트를 resStar로 사용
	if len(full) < 16 {
		return nil, fmt.Errorf("RES* length < 16: %d", len(full))
	}
	return full[len(full)-16:], nil
}

// deriveHResStar - Derive HRES* (HXRES*) according to 3GPP TS 33.501
func (u *UEAuth) deriveHResStar() []byte {
	// HRES* = SHA-256(RAND || RES*)[128 least significant bits]
	h := sha256.New()
	h.Write(u.RAND)
	h.Write(u.ResStar)
	hash := h.Sum(nil)

	// Take the least significant 128 bits (last 16 bytes)
	return hash[len(hash)-16:]
}

// CalculateResStarWrapper - Wrapper function to match your existing code
func CalculateResStar(opcStr, kStr string, rand, autn []byte, servingNetworkName string) (resStar, hresStar, ckPrime, ikPrime []byte, err error) {
	// Decode OPc and K from hex strings
	opc, err := hex.DecodeString(opcStr)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to decode OPc: %w", err)
	}

	k, err := hex.DecodeString(kStr)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to decode K: %w", err)
	}

	// Remove "5G:" prefix from serving network name if present
	if idx := strings.Index(servingNetworkName, ":"); idx >= 0 {
		servingNetworkName = servingNetworkName[idx+1:]
	}

	// Create UEAuth instance
	ueAuth := &UEAuth{
		K:           k,
		OPc:         opc,
		RAND:        rand,
		AUTN:        autn,
		ServingName: servingNetworkName,
	}

	// Calculate RES*
	err = ueAuth.CalculateResStar()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// For 5G, CK' and IK' are not directly used, instead KAUSF is used
	// But if you need them for compatibility, we can derive them
	ckPrime, ikPrime = deriveKeyPrimes(ueAuth.CK, ueAuth.IK, servingNetworkName, ueAuth.AK)

	return ueAuth.ResStar, ueAuth.HResStar, ckPrime, ikPrime, nil
}

// deriveKeyPrimes - Derive CK' and IK' for compatibility (if needed)
func deriveKeyPrimes(ck, ik []byte, servingNetworkName string, ak []byte) (ckPrime, ikPrime []byte) {
	// This is an optional function if you need CK' and IK'
	// In 5G, KAUSF is typically used instead

	// For now, return the first 16 bytes of KAUSF as CK' and next 16 as IK'
	// This is a simplified approach - adjust based on your needs
	fc := byte(0x69) // Example FC for key derivation

	var s bytes.Buffer
	s.WriteByte(fc)
	s.Write([]byte(servingNetworkName))

	key := append(ck, ik...)
	h := hmac.New(sha256.New, key)
	h.Write(s.Bytes())
	derived := h.Sum(nil)

	ckPrime = derived[:16]
	ikPrime = derived[16:32]

	return ckPrime, ikPrime
}

// KDFLen - Helper function to calculate length for KDF parameters
func KDFLen(p []byte) []byte {
	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(p)))
	return length
}

// DebugCompareWithExpected - Compare our results with expected values
func (u *UEAuth) DebugCompareWithExpected(expectedResStar, expectedHResStar string) {
	fmt.Printf("\n🔐 ========== DEBUG COMPARISON ==========\n")

	// The free5gc logs show hex-encoded strings, not double-encoded
	// So we just compare directly
	fmt.Printf("🔐 Expected RES*: %s\n", expectedResStar)
	fmt.Printf("🔐 Our RES*:      %s\n", hex.EncodeToString(u.ResStar))

	if expectedResStar == hex.EncodeToString(u.ResStar) {
		fmt.Printf("🔐 ✅ RES* MATCHES!\n")
	} else {
		fmt.Printf("🔐 ❌ RES* MISMATCH\n")

		// Try to decode in case it's double-encoded
		if decoded, err := hex.DecodeString(expectedResStar); err == nil {
			decodedStr := string(decoded)
			fmt.Printf("🔐 Expected RES* (decoded as ASCII): %s\n", decodedStr)

			if decodedStr == hex.EncodeToString(u.ResStar) {
				fmt.Printf("🔐 ✅ RES* MATCHES after decoding!\n")
			}
		}
	}

	fmt.Printf("\n🔐 Expected HRES*: %s\n", expectedHResStar)
	fmt.Printf("🔐 Our HRES*:      %s\n", hex.EncodeToString(u.HResStar))

	if expectedHResStar == hex.EncodeToString(u.HResStar) {
		fmt.Printf("🔐 ✅ HRES* MATCHES!\n")
	} else {
		fmt.Printf("🔐 ❌ HRES* MISMATCH\n")

		// Try to decode in case it's double-encoded
		if decoded, err := hex.DecodeString(expectedHResStar); err == nil {
			decodedStr := string(decoded)
			fmt.Printf("🔐 Expected HRES* (decoded as ASCII): %s\n", decodedStr)

			if decodedStr == hex.EncodeToString(u.HResStar) {
				fmt.Printf("🔐 ✅ HRES* MATCHES after decoding!\n")
			}
		}
	}
}
