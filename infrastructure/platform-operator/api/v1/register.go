package v1

func init() {
	SchemeBuilder.Register(&ApplicationClaim{}, &ApplicationClaimList{})
}
