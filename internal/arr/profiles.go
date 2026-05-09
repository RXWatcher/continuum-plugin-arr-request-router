package arr

import (
	"context"
	"fmt"
)

// ResolveRadarrDefaults fills in the root folder path and quality profile id
// from Radarr's configured defaults when the supplied values are blank/zero.
// Sonarr's caller has its own variant because of language profiles.
func ResolveRadarrDefaults(ctx context.Context, r *Radarr, root string, quality int) (string, int, error) {
	if root == "" {
		folders, err := r.RootFolders(ctx)
		if err != nil {
			return "", 0, fmt.Errorf("radarr rootfolders: %w", err)
		}
		if len(folders) == 0 {
			return "", 0, fmt.Errorf("radarr: no root folders configured")
		}
		root = folders[0].Path
	}
	if quality == 0 {
		profiles, err := r.QualityProfiles(ctx)
		if err != nil {
			return "", 0, fmt.Errorf("radarr qualityprofiles: %w", err)
		}
		if len(profiles) == 0 {
			return "", 0, fmt.Errorf("radarr: no quality profiles configured")
		}
		quality = profiles[0].ID
	}
	return root, quality, nil
}

// SonarrDefaults bundles all three Sonarr defaults so callers can fill them
// in one round trip.
type SonarrDefaults struct {
	RootFolderPath    string
	QualityProfileID  int
	LanguageProfileID int
}

// ResolveSonarrDefaults fills in any blank values from Sonarr.
func ResolveSonarrDefaults(ctx context.Context, s *Sonarr, want SonarrDefaults) (SonarrDefaults, error) {
	out := want
	if out.RootFolderPath == "" {
		folders, err := s.RootFolders(ctx)
		if err != nil {
			return SonarrDefaults{}, fmt.Errorf("sonarr rootfolders: %w", err)
		}
		if len(folders) == 0 {
			return SonarrDefaults{}, fmt.Errorf("sonarr: no root folders configured")
		}
		out.RootFolderPath = folders[0].Path
	}
	if out.QualityProfileID == 0 {
		profiles, err := s.QualityProfiles(ctx)
		if err != nil {
			return SonarrDefaults{}, fmt.Errorf("sonarr qualityprofiles: %w", err)
		}
		if len(profiles) == 0 {
			return SonarrDefaults{}, fmt.Errorf("sonarr: no quality profiles configured")
		}
		out.QualityProfileID = profiles[0].ID
	}
	if out.LanguageProfileID == 0 {
		profiles, err := s.LanguageProfiles(ctx)
		if err != nil {
			// Sonarr v4 dropped language profiles; treat 404/501 as benign.
			out.LanguageProfileID = 0
		} else if len(profiles) > 0 {
			out.LanguageProfileID = profiles[0].ID
		}
	}
	return out, nil
}
