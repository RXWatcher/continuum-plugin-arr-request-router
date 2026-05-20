package arr

import (
	"context"
	"fmt"
)

type RadarrDefaultsResolver interface {
	RootFolders(context.Context) ([]RootFolder, error)
	QualityProfiles(context.Context) ([]QualityProfile, error)
}

func ResolveRadarrDefaults(ctx context.Context, cli RadarrDefaultsResolver, root string, quality int) (string, int, error) {
	if root == "" {
		folders, err := cli.RootFolders(ctx)
		if err != nil {
			return "", 0, fmt.Errorf("radarr rootfolders: %w", err)
		}
		if len(folders) == 0 {
			return "", 0, fmt.Errorf("radarr: no root folders configured")
		}
		root = folders[0].Path
	}
	if quality == 0 {
		profiles, err := cli.QualityProfiles(ctx)
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

type SonarrDefaults struct {
	RootFolderPath    string
	QualityProfileID  int
	LanguageProfileID int
}

type SonarrDefaultsResolver interface {
	RootFolders(context.Context) ([]RootFolder, error)
	QualityProfiles(context.Context) ([]QualityProfile, error)
	LanguageProfiles(context.Context) ([]LanguageProfile, error)
}

func ResolveSonarrDefaults(ctx context.Context, cli SonarrDefaultsResolver, want SonarrDefaults) (SonarrDefaults, error) {
	out := want
	if out.RootFolderPath == "" {
		folders, err := cli.RootFolders(ctx)
		if err != nil {
			return SonarrDefaults{}, fmt.Errorf("sonarr rootfolders: %w", err)
		}
		if len(folders) == 0 {
			return SonarrDefaults{}, fmt.Errorf("sonarr: no root folders configured")
		}
		out.RootFolderPath = folders[0].Path
	}
	if out.QualityProfileID == 0 {
		profiles, err := cli.QualityProfiles(ctx)
		if err != nil {
			return SonarrDefaults{}, fmt.Errorf("sonarr qualityprofiles: %w", err)
		}
		if len(profiles) == 0 {
			return SonarrDefaults{}, fmt.Errorf("sonarr: no quality profiles configured")
		}
		out.QualityProfileID = profiles[0].ID
	}
	if out.LanguageProfileID == 0 {
		profiles, err := cli.LanguageProfiles(ctx)
		if err != nil {
			out.LanguageProfileID = 0
		} else if len(profiles) > 0 {
			out.LanguageProfileID = profiles[0].ID
		}
	}
	return out, nil
}
