package cmd

import (
	"testing"
)

func TestUnhideAdvanced_Off(t *testing.T) {
	t.Setenv("DX_ADVANCED", "")

	profileCmd.Hidden = true
	statusCmd.Hidden = true
	rootCmd.PersistentFlags().Lookup("profile").Hidden = true

	unhideAdvanced()

	if !profileCmd.Hidden {
		t.Error("profileCmd should stay hidden when DX_ADVANCED is not set")
	}
	if !statusCmd.Hidden {
		t.Error("statusCmd should stay hidden when DX_ADVANCED is not set")
	}
	if !rootCmd.PersistentFlags().Lookup("profile").Hidden {
		t.Error("--profile flag should stay hidden when DX_ADVANCED is not set")
	}
}

func TestUnhideAdvanced_On(t *testing.T) {
	t.Setenv("DX_ADVANCED", "1")

	profileCmd.Hidden = true
	statusCmd.Hidden = true
	rootCmd.PersistentFlags().Lookup("profile").Hidden = true

	unhideAdvanced()

	if profileCmd.Hidden {
		t.Error("profileCmd should be visible when DX_ADVANCED=1")
	}
	if statusCmd.Hidden {
		t.Error("statusCmd should be visible when DX_ADVANCED=1")
	}
	if rootCmd.PersistentFlags().Lookup("profile").Hidden {
		t.Error("--profile flag should be visible when DX_ADVANCED=1")
	}

	// Reset for other tests
	profileCmd.Hidden = true
	statusCmd.Hidden = true
	rootCmd.PersistentFlags().Lookup("profile").Hidden = true
}
