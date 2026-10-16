// Package compat holds upstream facts verified per docs/COMPATIBILITY.md.
// Keep this file and that document in sync (CC-1).
package compat

// Image is a pinned container image.
type Image struct {
	Repo   string
	Tag    string
	Digest string
}

// Ref returns the image reference pinned by tag and digest.
func (i Image) Ref() string {
	return i.Repo + ":" + i.Tag + "@" + i.Digest
}

// Pinned reports whether both tag and digest are recorded.
func (i Image) Pinned() bool {
	return i.Repo != "" && i.Tag != "" && i.Digest != ""
}

// Component describes one app as verified in docs/COMPATIBILITY.md.
type Component struct {
	Name  string
	Image Image
	// Port is the HTTP port inside the container.
	Port int
	// LinuxServer images take PUID/PGID env vars; others take Compose `user:`.
	LinuxServer bool
}

// Pins maps component name to its verified record.
type Pins map[string]Component

// Lookup returns the component and whether its image is fully pinned.
func (p Pins) Lookup(name string) (Component, bool) {
	c, ok := p[name]
	return c, ok && c.Image.Pinned()
}

// Default is the verified M0 core video stack (verified 2026-10-15).
var Default = Pins{
	"jellyfin": {
		Name: "jellyfin",
		Image: Image{
			Repo:   "jellyfin/jellyfin",
			Tag:    "12.2.20261005-225228",
			Digest: "sha256:357724bf0ae27a672c7cbaa899db2d9abeb13dbd8657ccce750258a4c059d037",
		},
		Port: 8096,
	},
	"radarr": {
		Name: "radarr",
		Image: Image{
			Repo:   "lscr.io/linuxserver/radarr",
			Tag:    "6.4.4.10685-ls321",
			Digest: "sha256:ea6822c956d26db648b237919f41340318f32e5b7dcc4ba702cf256a0f5f3c12",
		},
		Port:        7878,
		LinuxServer: true,
	},
	"sonarr": {
		Name: "sonarr",
		Image: Image{
			Repo:   "lscr.io/linuxserver/sonarr",
			Tag:    "4.0.20.3014-ls329",
			Digest: "sha256:44b06045a98bc1f237b9ffefc4b28dab64cb29305e4a2a94875ae93b5224e4cc",
		},
		Port:        8989,
		LinuxServer: true,
	},
	"prowlarr": {
		Name: "prowlarr",
		Image: Image{
			Repo:   "lscr.io/linuxserver/prowlarr",
			Tag:    "2.6.5.5623-ls163",
			Digest: "sha256:f9151e5bc1025c6d0a630d503210cdcb6bb55a7cc098562609d96a408d838902",
		},
		Port:        9696,
		LinuxServer: true,
	},
}
