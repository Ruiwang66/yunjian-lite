//go:build windows

package privatefs

import (
	"golang.org/x/sys/windows"
	"path/filepath"
	"testing"
)

func TestWindowsRejectsReadableEveryoneACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	if err := AtomicWrite(path, []byte("secret")); err != nil {
		t.Fatal(err)
	}
	token, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + token.User.Sid.String() + ")(A;;FR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecret(path); err == nil {
		t.Fatal("Everyone-readable secret accepted")
	}
	if err := File(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecret(path); err != nil {
		t.Fatal("private ACL not restored", err)
	}
}
