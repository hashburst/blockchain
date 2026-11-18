package consensus

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"testing"
)

const r6MembershipStateDomain = "HASHBURST_VALIDATOR_MEMBERSHIP_STATE_V1"

type r6MembershipRootVector struct {
	name     string
	version  uint64
	consumed []string
	preimage string
	root     string
}

func r6VectorPutUint64(b *bytes.Buffer, v uint64) {
	var x [8]byte
	binary.BigEndian.PutUint64(x[:], v)
	b.Write(x[:])
}

func r6VectorPutString(b *bytes.Buffer, s string) {
	r6VectorPutUint64(b, uint64(len(s)))
	b.WriteString(s)
}

func r6MembershipVectorPreimage(version uint64, consumed []string) []byte {
	ordered := append([]string(nil), consumed...)
	sort.Strings(ordered)

	var b bytes.Buffer
	r6VectorPutString(&b, r6MembershipStateDomain)
	r6VectorPutUint64(&b, version)
	r6VectorPutUint64(&b, uint64(len(ordered)))

	for _, h := range ordered {
		r6VectorPutString(&b, h)
	}

	return b.Bytes()
}

func r6MembershipRootVectors() []r6MembershipRootVector {
	return []r6MembershipRootVector{
		{
			name:     "V0_empty_version_zero",
			version:  0,
			consumed: nil,
			preimage: "00000000000000274841534842555253545f56414c494441544f525f4d454d424552534849505f53544154455f563100000000000000000000000000000000",
			root:     "0x427ab3c089042bf8a5045369baa635787973c9cb8d12e273a078cff670b4f9e4",
		},
		{
			name:     "V1_nonzero_version_empty",
			version:  7,
			consumed: nil,
			preimage: "00000000000000274841534842555253545f56414c494441544f525f4d454d424552534849505f53544154455f563100000000000000070000000000000000",
			root:     "0x0dcce93e283a3de9d6fdd88be19672fb25afc2334f376760194f837ec58e18de",
		},
		{
			name:    "V2_single_consumed",
			version: 1,
			consumed: []string{
				"0x1111111111111111111111111111111111111111111111111111111111111111",
			},
			preimage: "00000000000000274841534842555253545f56414c494441544f525f4d454d424552534849505f53544154455f5631000000000000000100000000000000010000000000000042307831313131313131313131313131313131313131313131313131313131313131313131313131313131313131313131313131313131313131313131313131313131",
			root:     "0xf9e3ce918f8209550dea85de37710de8081906a20e13c1b4a8bd2420bf993588",
		},
		{
			name:    "V3_multiple_consumed_sorted",
			version: 9,
			consumed: []string{
				"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
				"0x0000000000000000000000000000000000000000000000000000000000000000",
				"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			preimage: "00000000000000274841534842555253545f56414c494441544f525f4d454d424552534849505f53544154455f563100000000000000090000000000000003000000000000004230783030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303000000000000000423078616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161610000000000000042307866666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666",
			root:     "0x167b32e5b720242baf81adc4f75136a3ff1ea2f3469dbcda1b0efe08ef5f9ad9",
		},
	}
}

func TestR6MembershipStateRootByteVectors(t *testing.T) {
	for _, tc := range r6MembershipRootVectors() {
		t.Run(tc.name, func(t *testing.T) {
			preimage := r6MembershipVectorPreimage(tc.version, tc.consumed)

			gotPreimage := hex.EncodeToString(preimage)
			if gotPreimage != tc.preimage {
				t.Fatalf(
					"canonical membership preimage mismatch:\ngot:  %s\nwant: %s",
					gotPreimage,
					tc.preimage,
				)
			}

			sum := sha256.Sum256(preimage)
			gotRoot := "0x" + hex.EncodeToString(sum[:])

			if gotRoot != tc.root {
				t.Fatalf(
					"canonical membership root vector mismatch: got=%s want=%s",
					gotRoot,
					tc.root,
				)
			}

			snapshot := MembershipStateSnapshot{
				Version:  tc.version,
				Consumed: append([]string(nil), tc.consumed...),
			}

			state, err := RestoreMembershipState(snapshot)
			if err != nil {
				t.Fatalf("RestoreMembershipState: %v", err)
			}

			if got := state.Root(); got != tc.root {
				t.Fatalf(
					"MembershipState.Root mismatch: got=%s want=%s",
					got,
					tc.root,
				)
			}
		})
	}
}

func TestR6MembershipStateRootInsertionOrderInvariantVector(t *testing.T) {
	a, err := RestoreMembershipState(MembershipStateSnapshot{
		Version: 9,
		Consumed: []string{
			"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0x0000000000000000000000000000000000000000000000000000000000000000",
			"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	})
	if err != nil {
		t.Fatalf("restore A: %v", err)
	}

	b, err := RestoreMembershipState(MembershipStateSnapshot{
		Version: 9,
		Consumed: []string{
			"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0x0000000000000000000000000000000000000000000000000000000000000000",
		},
	})
	if err != nil {
		t.Fatalf("restore B: %v", err)
	}

	const want = "0x167b32e5b720242baf81adc4f75136a3ff1ea2f3469dbcda1b0efe08ef5f9ad9"

	if a.Root() != want {
		t.Fatalf("A root mismatch: got=%s want=%s", a.Root(), want)
	}

	if b.Root() != want {
		t.Fatalf("B root mismatch: got=%s want=%s", b.Root(), want)
	}

	if a.Root() != b.Root() {
		t.Fatalf("same logical membership state produced different roots")
	}
}
