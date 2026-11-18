package consensus

import (
	"strings"
	"testing"
)

func TestR6MembershipStateRootDeterministic(t *testing.T) {
	a, err := RestoreMembershipState(MembershipStateSnapshot{
		Version:  7,
		Consumed: []string{strings.Repeat("bb", 32), strings.Repeat("aa", 32)},
	})
	if err != nil {
		t.Fatal(err)
	}

	b, err := RestoreMembershipState(MembershipStateSnapshot{
		Version:  7,
		Consumed: []string{strings.Repeat("aa", 32), strings.Repeat("bb", 32)},
	})
	if err != nil {
		t.Fatal(err)
	}

	if a.Root() != b.Root() {
		t.Fatalf("membership root is not deterministic: %s != %s", a.Root(), b.Root())
	}
}

func TestR6MembershipStateRootCommitsVersion(t *testing.T) {
	a, err := RestoreMembershipState(MembershipStateSnapshot{
		Version:  1,
		Consumed: []string{strings.Repeat("aa", 32)},
	})
	if err != nil {
		t.Fatal(err)
	}

	b, err := RestoreMembershipState(MembershipStateSnapshot{
		Version:  2,
		Consumed: []string{strings.Repeat("aa", 32)},
	})
	if err != nil {
		t.Fatal(err)
	}

	if a.Root() == b.Root() {
		t.Fatal("membership root does not commit version")
	}
}

func TestR6MembershipStateRootCommitsConsumed(t *testing.T) {
	a, err := RestoreMembershipState(MembershipStateSnapshot{
		Version:  1,
		Consumed: []string{strings.Repeat("aa", 32)},
	})
	if err != nil {
		t.Fatal(err)
	}

	b, err := RestoreMembershipState(MembershipStateSnapshot{
		Version: 1,
		Consumed: []string{
			strings.Repeat("aa", 32),
			strings.Repeat("bb", 32),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if a.Root() == b.Root() {
		t.Fatal("membership root does not commit consumed proposals")
	}
}

func TestR6MembershipStateRootCanonicalShape(t *testing.T) {
	s := NewMembershipState(0)

	root := s.Root()

	got := strings.TrimPrefix(root, "0x")
	if len(got) != 64 {
		t.Fatalf("membership root must be 32 bytes: %q", root)
	}
}
