package v1

func init() {
	SchemeBuilder.Register(
		&ApplicationClaim{}, &ApplicationClaimList{},
		&BootstrapClaim{}, &BootstrapClaimList{},
		&PlatformClaim{}, &PlatformClaimList{},
	)
}
