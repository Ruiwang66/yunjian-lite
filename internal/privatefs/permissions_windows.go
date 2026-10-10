//go:build windows

package privatefs

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"unsafe"
)

func protect(path string, dir bool) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	inheritance := ""
	if dir {
		inheritance = "OICI"
	}
	sddl := "D:P(A;" + inheritance + ";FA;;;" + user.User.Sid.String() + ")(A;" + inheritance + ";FA;;;SY)(A;" + inheritance + ";FA;;;BA)"
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
func readableOnlyByOwners(path string, _ os.FileInfo) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if acl == nil {
		return fmt.Errorf("secret file has a permissive ACL")
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	current, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	allowed := map[string]bool{owner.String(): true, current.User.Sid.String(): true, "S-1-5-18": true, "S-1-5-19": true, "S-1-5-32-544": true}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var header *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &header); err != nil {
			return err
		}
		if header.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE { continue }
        if header.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE { return fmt.Errorf("secret file has an unsupported ACL entry") }
		ace := (*windows.ACCESS_ALLOWED_ACE)(unsafe.Pointer(header))
		if ace.Mask&(windows.FILE_READ_DATA|windows.GENERIC_READ|windows.GENERIC_ALL) == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !allowed[sid.String()] {
			return fmt.Errorf("secret file grants read access to another account; restrict its ACL")
		}
	}
	return nil
}
