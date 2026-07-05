package servingstate

type RetentionPolicy struct {
	ProtectActive              bool
	ProtectDraining            bool
	RequireApplyForDestructive bool
}

func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		ProtectActive:              true,
		ProtectDraining:            false,
		RequireApplyForDestructive: true,
	}
}
