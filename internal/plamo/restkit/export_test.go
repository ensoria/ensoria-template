package restkit

// InitialStrictDeclarations reports the default the package gave the strict
// declaration mode when it was initialised.
//
// The suite cannot read that default off StrictDeclarations: the specs that
// exercise both modes write to the flag, so by the time one of them asks, the
// value is whatever the last spec left behind. The default is what carries the
// guarantee that a test binary checks declarations at all, so it is asserted on
// directly.
func InitialStrictDeclarations() bool { return initialStrictDeclarations }
