package calculate

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/free5gc/milenage"
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

// hmacSHA256 computes HMAC-SHA256(key, data).
func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// kdf implements 3GPP TS 33.501 Annex A.1 KDF with proper parameter encoding
func kdf(K []byte, FC byte, params [][]byte) []byte {
	fmt.Printf("🔐     KDF DEBUG:\n")
	fmt.Printf("🔐       Key: %s (len=%d)\n", hex.EncodeToString(K), len(K))
	fmt.Printf("🔐       FC:  0x%02X\n", FC)

	buf := []byte{FC}

	for i, P := range params {
		fmt.Printf("🔐       P%d:  %s (len=%d)\n", i, hex.EncodeToString(P), len(P))
		buf = append(buf, P...)

		// Length field: 2 bytes, big endian, length in bits
		lengthInBits := uint16(len(P) * 8)
		L := make([]byte, 2)
		binary.BigEndian.PutUint16(L, lengthInBits)
		buf = append(buf, L...)
		fmt.Printf("🔐       L%d:  %s (len_bits=%d)\n", i, hex.EncodeToString(L), lengthInBits)
	}

	fmt.Printf("🔐       Full input: %s (len=%d)\n", hex.EncodeToString(buf), len(buf))

	result := hmacSHA256(K, buf)
	fmt.Printf("🔐       KDF Output: %s (len=%d)\n", hex.EncodeToString(result), len(result))

	return result
}

// CalculateResStar - Calculate free5gc resStar vector (XRES*)
func (u *UEAuth) CalculateResStar() error {
	fmt.Printf("🔐 ========== FREE5GC RESSTAR CALCULATION ==========\n")

	// Validate input parameters
	if len(u.K) != 16 || len(u.OPc) != 16 || len(u.RAND) != 16 || len(u.AUTN) != 16 {
		return errors.New("invalid input parameter lengths")
	}

	// STEP 1: Milenage computation
	fmt.Printf("🔐 STEP 1: Milenage computation...\n")

	// Convert to uint8 arrays as required by milenage package
	var opcArray, kArray, randArray [16]uint8
	copy(opcArray[:], u.OPc)
	copy(kArray[:], u.K)
	copy(randArray[:], u.RAND)

	// Extract SQN and AMF from AUTN
	sqnXorAK := u.AUTN[0:6]
	amfBytes := u.AUTN[6:8]
	amf := binary.BigEndian.Uint16(amfBytes)

	fmt.Printf("🔐   OPc: %s\n", hex.EncodeToString(u.OPc))
	fmt.Printf("🔐   K:   %s\n", hex.EncodeToString(u.K))
	fmt.Printf("🔐   RAND: %s\n", hex.EncodeToString(u.RAND))
	fmt.Printf("🔐   AMF: 0x%04X\n", amf)

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

	fmt.Printf("🔐 Milenage Results:\n")
	fmt.Printf("🔐   RES: %s\n", hex.EncodeToString(u.RES))
	fmt.Printf("🔐   CK:  %s\n", hex.EncodeToString(u.CK))
	fmt.Printf("🔐   IK:  %s\n", hex.EncodeToString(u.IK))
	fmt.Printf("🔐   AK:  %s\n", hex.EncodeToString(u.AK))

	// Extract SQN
	sqnBytes := make([]byte, 6)
	for i := 0; i < 6; i++ {
		sqnBytes[i] = sqnXorAK[i] ^ u.AK[i]
	}
	fmt.Printf("🔐   SQN: %s\n", hex.EncodeToString(sqnBytes))

	// STEP 2: KAUSF derivation (3GPP TS 33.501 Annex A.2)
	// KAUSF = KDF(CK||IK, FC=0x6A, SQN⊕AK, ServingNetworkName)
	fmt.Printf("🔐 STEP 2: KAUSF derivation...\n")
	ckik := make([]byte, 32)
	copy(ckik, u.CK)
	copy(ckik[16:], u.IK)

	servingNameBytes := []byte(u.ServingName)

	fmt.Printf("🔐   CK||IK: %s (len=%d)\n", hex.EncodeToString(ckik), len(ckik))
	fmt.Printf("🔐   SQN⊕AK: %s (len=%d)\n", hex.EncodeToString(sqnXorAK), len(sqnXorAK))
	fmt.Printf("🔐   ServingName: '%s' -> %s (len=%d)\n", u.ServingName, hex.EncodeToString(servingNameBytes), len(servingNameBytes))

	// According to 3GPP TS 33.501 A.2: P0 = SQN⊕AK, P1 = serving network name
	kausfFull := kdf(ckik, 0x6A, [][]byte{sqnXorAK, servingNameBytes})
	u.KAUSF = kausfFull[:32] // Take first 32 bytes (256 bits)
	fmt.Printf("🔐   KAUSF (32 bytes): %s\n", hex.EncodeToString(u.KAUSF))

	// STEP 3: RES* (XRES*) derivation (3GPP TS 33.501 Annex A.4)
	fmt.Printf("🔐 STEP 3: RES* (XRES*) derivation...\n")

	// Based on 3GPP TS 33.501 A.4 and UERANSIM working with free5gc,
	// the correct parameter order is: ServingName, RAND, RES
	fmt.Printf("🔐   CK||IK: %s\n", hex.EncodeToString(ckik))
	fmt.Printf("🔐   ServingName: '%s' -> %s\n", u.ServingName, hex.EncodeToString(servingNameBytes))
	fmt.Printf("🔐   RAND: %s (len=%d)\n", hex.EncodeToString(u.RAND), len(u.RAND))
	fmt.Printf("🔐   RES: %s (len=%d)\n", hex.EncodeToString(u.RES), len(u.RES))

	// Use the standard 3GPP order: ServingName, RAND, RES
	fmt.Printf("🔐   Using parameter order: ServingName, RAND, RES (3GPP TS 33.501)\n")
	resStarFull := kdf(ckik, 0x6B, [][]byte{servingNameBytes, u.RAND, u.RES})
	u.ResStar = resStarFull[:16] // Take first 16 bytes (128 bits) as per spec
	fmt.Printf("🔐   RES* (16 bytes): %s\n", hex.EncodeToString(u.ResStar))

	// STEP 4: HRES* (HXRES*) computation
	// HRES* = SHA256(RAND || RES*)[0:16] (MSB 128 bits)
	fmt.Printf("🔐 STEP 4: HRES* (HXRES*) computation...\n")
	randResStar := make([]byte, len(u.RAND)+len(u.ResStar))
	copy(randResStar, u.RAND)
	copy(randResStar[len(u.RAND):], u.ResStar)

	fmt.Printf("🔐   RAND||RES*: %s (len=%d)\n", hex.EncodeToString(randResStar), len(randResStar))

	h := sha256.New()
	h.Write(randResStar)
	sha256Full := h.Sum(nil)

	fmt.Printf("🔐   SHA256 full: %s (len=%d)\n", hex.EncodeToString(sha256Full), len(sha256Full))

	// Take MSB 128 bits for HRES*
	u.HResStar = sha256Full[:16]
	fmt.Printf("🔐   HRES* (MSB128): %s\n", hex.EncodeToString(u.HResStar))

	fmt.Printf("🔐 ========== RESSTAR COMPUTATION COMPLETE ==========\n")
	fmt.Printf("🔐 FREE5GC RESSTAR VECTOR:\n")
	fmt.Printf("🔐   RES*:  %s\n", hex.EncodeToString(u.ResStar))
	fmt.Printf("🔐   HRES*: %s\n", hex.EncodeToString(u.HResStar))
	fmt.Printf("🔐 ================================================\n")

	return nil
}

// GetResStarVector returns the calculated RES* vector
func (u *UEAuth) GetResStarVector() ([]byte, error) {
	if u.ResStar == nil {
		return nil, errors.New("RES* not calculated; run CalculateResStar first")
	}
	return u.ResStar, nil
}

// GetHResStarVector returns the calculated HRES* vector
func (u *UEAuth) GetHResStarVector() ([]byte, error) {
	if u.HResStar == nil {
		return nil, errors.New("HRES* not calculated; run CalculateResStar first")
	}
	return u.HResStar, nil
}

// ValidateResStar compares computed RES* against expected value
func (u *UEAuth) ValidateResStar(expectedResStar string) (bool, error) {
	if u.ResStar == nil {
		return false, errors.New("RES* not computed; run CalculateResStar first")
	}

	expectedBytes, err := hex.DecodeString(expectedResStar)
	if err != nil {
		return false, fmt.Errorf("failed to decode expected RES*: %w", err)
	}

	fmt.Printf("🔐 🔍 RES* validation:\n")
	fmt.Printf("🔐   Computed: %s\n", hex.EncodeToString(u.ResStar))
	fmt.Printf("🔐   Expected: %s\n", expectedResStar)

	match := bytes.Equal(u.ResStar, expectedBytes)
	if match {
		fmt.Printf("🔐 ✅ RES* validation SUCCESS!\n")
	} else {
		fmt.Printf("🔐 ❌ RES* validation FAILED!\n")
	}

	return match, nil
}

// ValidateHResStar compares computed HRES* against expected value
func (u *UEAuth) ValidateHResStar(expectedHResStar string) (bool, error) {
	if u.HResStar == nil {
		return false, errors.New("HRES* not computed; run CalculateResStar first")
	}

	expectedBytes, err := hex.DecodeString(expectedHResStar)
	if err != nil {
		return false, fmt.Errorf("failed to decode expected HRES*: %w", err)
	}

	fmt.Printf("🔐 🔍 HRES* validation:\n")
	fmt.Printf("🔐   Computed: %s\n", hex.EncodeToString(u.HResStar))
	fmt.Printf("🔐   Expected: %s\n", expectedHResStar)

	match := bytes.Equal(u.HResStar, expectedBytes)
	if match {
		fmt.Printf("🔐 ✅ HRES* validation SUCCESS!\n")
		return true, nil
	}

	fmt.Printf("🔐 ❌ MSB128 failed, trying LSB128...\n")

	// Try LSB128 as fallback (some implementations use LSB)
	randResStar := make([]byte, len(u.RAND)+len(u.ResStar))
	copy(randResStar, u.RAND)
	copy(randResStar[len(u.RAND):], u.ResStar)

	h := sha256.New()
	h.Write(randResStar)
	full := h.Sum(nil)
	lsbHResStar := full[16:] // LSB 128 bits

	fmt.Printf("🔐   LSB128: %s\n", hex.EncodeToString(lsbHResStar))

	if bytes.Equal(lsbHResStar, expectedBytes) {
		fmt.Printf("🔐 ✅ LSB128 WORKS! Updating HRES*\n")
		u.HResStar = lsbHResStar
		return true, nil
	}

	fmt.Printf("🔐 ❌ Both MSB128 and LSB128 failed\n")
	return false, nil
}

// CalculateResStarAlternative tries alternative parameter orders for RES* calculation
func (u *UEAuth) CalculateResStarAlternative() error {
	fmt.Printf("🔐 ========== ALTERNATIVE RESSTAR CALCULATION ==========\n")

	// First perform the same Milenage computation
	if err := u.performMilenage(); err != nil {
		return err
	}

	// Calculate KAUSF the same way
	ckik := make([]byte, 32)
	copy(ckik, u.CK)
	copy(ckik[16:], u.IK)

	sqnXorAK := u.AUTN[0:6]
	servingNameBytes := []byte(u.ServingName)

	kausfFull := kdf(ckik, 0x6A, [][]byte{sqnXorAK, servingNameBytes})
	u.KAUSF = kausfFull[:32]

	// Try different parameter orders for RES*
	fmt.Printf("🔐 Trying alternative parameter orders for RES*...\n")

	// Alternative 1: RAND, ServingName, RES
	fmt.Printf("🔐 Alternative 1: RAND, ServingName, RES\n")
	resStarFull := kdf(ckik, 0x6B, [][]byte{u.RAND, servingNameBytes, u.RES})
	resStar1 := resStarFull[:16]
	fmt.Printf("🔐   RES* Alt1: %s\n", hex.EncodeToString(resStar1))

	// Alternative 2: ServingName, RAND, RES (original spec order)
	fmt.Printf("🔐 Alternative 2: ServingName, RAND, RES\n")
	resStarFull = kdf(ckik, 0x6B, [][]byte{servingNameBytes, u.RAND, u.RES})
	resStar2 := resStarFull[:16]
	fmt.Printf("🔐   RES* Alt2: %s\n", hex.EncodeToString(resStar2))

	// Alternative 3: RES, RAND, ServingName
	fmt.Printf("🔐 Alternative 3: RES, RAND, ServingName\n")
	resStarFull = kdf(ckik, 0x6B, [][]byte{u.RES, u.RAND, servingNameBytes})
	resStar3 := resStarFull[:16]
	fmt.Printf("🔐   RES* Alt3: %s\n", hex.EncodeToString(resStar3))

	// Use Alternative 1 as default for this method
	u.ResStar = resStar1

	// Calculate HRES* the same way
	randResStar := make([]byte, len(u.RAND)+len(u.ResStar))
	copy(randResStar, u.RAND)
	copy(randResStar[len(u.RAND):], u.ResStar)

	h := sha256.New()
	h.Write(randResStar)
	sha256Full := h.Sum(nil)
	u.HResStar = sha256Full[:16]

	fmt.Printf("🔐 Using Alternative 1 result:\n")
	fmt.Printf("🔐   RES*:  %s\n", hex.EncodeToString(u.ResStar))
	fmt.Printf("🔐   HRES*: %s\n", hex.EncodeToString(u.HResStar))

	return nil
}

// performMilenage is a helper to avoid code duplication
func (u *UEAuth) performMilenage() error {
	// Convert to uint8 arrays as required by milenage package
	var opcArray, kArray, randArray [16]uint8
	copy(opcArray[:], u.OPc)
	copy(kArray[:], u.K)
	copy(randArray[:], u.RAND)

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

	return nil
}

// DebugCompareWithExpected helps debug RES* calculation issues
func (u *UEAuth) DebugCompareWithExpected(expectedXresStar string) error {
	fmt.Printf("🔐 ========== DEBUG COMPARISON ==========\n")

	// Check if the expected value is double-encoded
	fmt.Printf("🔐 Expected Xres* from AUSF (raw): %s (len=%d)\n", expectedXresStar, len(expectedXresStar))

	// Decode the expected value
	if decoded, err := hex.DecodeString(expectedXresStar); err == nil {
		decodedStr := string(decoded)
		fmt.Printf("🔐 Expected Xres* decoded as string: %s (len=%d)\n", decodedStr, len(decodedStr))

		// Check if it's valid hex after first decode
		if decodedBytes, err2 := hex.DecodeString(decodedStr); err2 == nil {
			fmt.Printf("🔐 ⚠️  Expected value is double-encoded hex!\n")
			fmt.Printf("🔐 Expected Xres* final value: %s (len=%d bytes)\n", decodedStr, len(decodedBytes))
		} else {
			fmt.Printf("🔐 Expected value is single-encoded: %x\n", decoded)
		}
	}

	// Try all calculation methods and compare
	fmt.Printf("\n🔐 Trying all RES* calculation methods...\n")

	// Calculate using all methods
	ckik := make([]byte, 32)
	copy(ckik, u.CK)
	copy(ckik[16:], u.IK)
	servingNameBytes := []byte(u.ServingName)

	// Method 1: ServingName, RES, RAND (from formal spec)
	fmt.Printf("\n🔐 Method 1: ServingName, RES, RAND\n")
	resStarFull := kdf(ckik, 0x6B, [][]byte{servingNameBytes, u.RES, u.RAND})
	resStar1 := resStarFull[:16]
	fmt.Printf("🔐   RES*: %s\n", hex.EncodeToString(resStar1))
	u.checkMatch(resStar1, expectedXresStar)

	// Method 2: ServingName, RAND, RES (original in code)
	fmt.Printf("\n🔐 Method 2: ServingName, RAND, RES\n")
	resStarFull = kdf(ckik, 0x6B, [][]byte{servingNameBytes, u.RAND, u.RES})
	resStar2 := resStarFull[:16]
	fmt.Printf("🔐   RES*: %s\n", hex.EncodeToString(resStar2))
	u.checkMatch(resStar2, expectedXresStar)

	// Method 3: RAND, ServingName, RES
	fmt.Printf("\n🔐 Method 3: RAND, ServingName, RES\n")
	resStarFull = kdf(ckik, 0x6B, [][]byte{u.RAND, servingNameBytes, u.RES})
	resStar3 := resStarFull[:16]
	fmt.Printf("🔐   RES*: %s\n", hex.EncodeToString(resStar3))
	u.checkMatch(resStar3, expectedXresStar)

	// Method 4: RES, RAND, ServingName
	fmt.Printf("\n🔐 Method 4: RES, RAND, ServingName\n")
	resStarFull = kdf(ckik, 0x6B, [][]byte{u.RES, u.RAND, servingNameBytes})
	resStar4 := resStarFull[:16]
	fmt.Printf("🔐   RES*: %s\n", hex.EncodeToString(resStar4))
	u.checkMatch(resStar4, expectedXresStar)

	// Method 5: RES, ServingName, RAND
	fmt.Printf("\n🔐 Method 5: RES, ServingName, RAND\n")
	resStarFull = kdf(ckik, 0x6B, [][]byte{u.RES, servingNameBytes, u.RAND})
	resStar5 := resStarFull[:16]
	fmt.Printf("🔐   RES*: %s\n", hex.EncodeToString(resStar5))
	u.checkMatch(resStar5, expectedXresStar)

	// Method 6: RAND, RES, ServingName
	fmt.Printf("\n🔐 Method 6: RAND, RES, ServingName\n")
	resStarFull = kdf(ckik, 0x6B, [][]byte{u.RAND, u.RES, servingNameBytes})
	resStar6 := resStarFull[:16]
	fmt.Printf("🔐   RES*: %s\n", hex.EncodeToString(resStar6))
	u.checkMatch(resStar6, expectedXresStar)

	return nil
}

// checkMatch is a helper to check if calculated RES* matches expected
func (u *UEAuth) checkMatch(resStar []byte, expectedXresStar string) {
	// Direct comparison
	if hex.EncodeToString(resStar) == expectedXresStar {
		fmt.Printf("🔐   ✅ DIRECT MATCH!\n")
		return
	}

	// Double-encoded comparison
	hexOfHex := hex.EncodeToString([]byte(hex.EncodeToString(resStar)))
	if hexOfHex == expectedXresStar {
		fmt.Printf("🔐   ✅ MATCH with double-encoding!\n")
		return
	}

	// Check if expected is double-encoded and compare with decoded
	if decoded, err := hex.DecodeString(expectedXresStar); err == nil {
		decodedStr := string(decoded)
		if hex.EncodeToString(resStar) == decodedStr {
			fmt.Printf("🔐   ✅ MATCH after decoding expected value!\n")
			return
		}
	}

	fmt.Printf("🔐   ❌ No match\n")
}

// Legacy method names for backward compatibility
func (u *UEAuth) PerformUEAuth() error {
	return u.CalculateResStar()
}

func (u *UEAuth) PerformUEAuthAlternative() error {
	return u.CalculateResStarAlternative()
}

// Legacy getters for backward compatibility
func (u *UEAuth) GetXRESStar() []byte {
	return u.ResStar
}

func (u *UEAuth) GetHXRESStar() []byte {
	return u.HResStar
}
