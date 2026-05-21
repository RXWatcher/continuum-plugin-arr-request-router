package arr_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/arr"
)

func TestIsConflict(t *testing.T) {
	if arr.IsConflict(errors.New("unexpected response 409")) {
		t.Fatal("unexpected conflict detection from plain error text")
	}
	if !arr.IsConflict(&arr.HTTPError{StatusCode: http.StatusConflict}) {
		t.Fatal("expected conflict detection from HTTPError")
	}
	if arr.IsConflict(errors.New("unexpected response 500")) {
		t.Fatal("unexpected non-conflict detection")
	}
}

type fakeRadarr struct {
	rootFolders     []arr.RootFolder
	rootFoldersErr  error
	qualityProfiles []arr.QualityProfile
	qualityErr      error
}

func (f fakeRadarr) RootFolders(context.Context) ([]arr.RootFolder, error) {
	return f.rootFolders, f.rootFoldersErr
}

func (f fakeRadarr) QualityProfiles(context.Context) ([]arr.QualityProfile, error) {
	return f.qualityProfiles, f.qualityErr
}

type fakeSonarr struct {
	rootFolders      []arr.RootFolder
	rootFoldersErr   error
	qualityProfiles  []arr.QualityProfile
	qualityErr       error
	languageProfiles []arr.LanguageProfile
	languageErr      error
}

func (f fakeSonarr) RootFolders(context.Context) ([]arr.RootFolder, error) {
	return f.rootFolders, f.rootFoldersErr
}

func (f fakeSonarr) QualityProfiles(context.Context) ([]arr.QualityProfile, error) {
	return f.qualityProfiles, f.qualityErr
}

func (f fakeSonarr) LanguageProfiles(context.Context) ([]arr.LanguageProfile, error) {
	return f.languageProfiles, f.languageErr
}

func TestResolveRadarrDefaultsUsesConfiguredValues(t *testing.T) {
	cli := fakeRadarr{}
	root, profileID, err := arr.ResolveRadarrDefaults(context.Background(), cli, "/movies", 7)
	if err != nil {
		t.Fatalf("ResolveRadarrDefaults: %v", err)
	}
	if root != "/movies" || profileID != 7 {
		t.Fatalf("got root=%q profile=%d", root, profileID)
	}
}

func TestResolveRadarrDefaultsFallsBackToFirstConfiguredValues(t *testing.T) {
	cli := fakeRadarr{
		rootFolders:     []arr.RootFolder{{ID: 1, Path: "/movies"}},
		qualityProfiles: []arr.QualityProfile{{ID: 7, Name: "HD"}},
	}
	root, profileID, err := arr.ResolveRadarrDefaults(context.Background(), cli, "", 0)
	if err != nil {
		t.Fatalf("ResolveRadarrDefaults: %v", err)
	}
	if root != "/movies" || profileID != 7 {
		t.Fatalf("got root=%q profile=%d", root, profileID)
	}
}

func TestResolveRadarrDefaultsErrorsWithoutConfiguredFallbacks(t *testing.T) {
	_, _, err := arr.ResolveRadarrDefaults(context.Background(), fakeRadarr{
		rootFoldersErr: errors.New("boom"),
	}, "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveSonarrDefaultsFallsBackToConfiguredValues(t *testing.T) {
	cli := fakeSonarr{
		rootFolders:      []arr.RootFolder{{ID: 1, Path: "/tv"}},
		qualityProfiles:  []arr.QualityProfile{{ID: 7}},
		languageProfiles: []arr.LanguageProfile{{ID: 3}},
	}
	got, err := arr.ResolveSonarrDefaults(context.Background(), cli, arr.SonarrDefaults{})
	if err != nil {
		t.Fatalf("ResolveSonarrDefaults: %v", err)
	}
	if got.RootFolderPath != "/tv" || got.QualityProfileID != 7 || got.LanguageProfileID != 3 {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveSonarrDefaultsIgnoresLanguageProfileErrors(t *testing.T) {
	got, err := arr.ResolveSonarrDefaults(context.Background(), fakeSonarr{
		rootFolders:     []arr.RootFolder{{ID: 1, Path: "/tv"}},
		qualityProfiles: []arr.QualityProfile{{ID: 7}},
		languageErr:     errors.New("not supported"),
	}, arr.SonarrDefaults{})
	if err != nil {
		t.Fatalf("ResolveSonarrDefaults: %v", err)
	}
	if got.LanguageProfileID != 0 {
		t.Fatalf("LanguageProfileID = %d, want 0", got.LanguageProfileID)
	}
}
