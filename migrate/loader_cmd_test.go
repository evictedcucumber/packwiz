package migrate

import (
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/jarcoal/httpmock"
)

const neoForgeMetadataFixture = `<metadata>
	<groupId>net.neoforged</groupId>
	<artifactId>neoforge</artifactId>
	<versioning>
		<versions>
			<version>21.1.200</version>
			<version>21.1.213</version>
		</versions>
	</versioning>
</metadata>`

func TestLoaderCommandExplicitVersion(t *testing.T) {
	httpmock.Activate(t)
	cmdtest.Chdir(t)
	cmdtest.WritePackFile(t, core.Pack{
		Name: "Test Pack", PackFormat: core.CurrentPackFormat,
		Versions: map[string]string{"minecraft": "1.21.1", "neoforge": "21.1.200"},
	})

	httpmock.RegisterResponder("GET", "https://maven.neoforged.net/releases/net/neoforged/neoforge/maven-metadata.xml",
		httpmock.NewStringResponder(200, neoForgeMetadataFixture))

	out := cmdtest.CaptureStdout(t, func() {
		loaderCommand.Run(loaderCommand, []string{"21.1.213"})
	})
	if !strings.Contains(out, "Updated NeoForge to version 21.1.213") {
		t.Errorf("output = %q, want confirmation of the version update", out)
	}

	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	if pack.Versions["neoforge"] != "21.1.213" {
		t.Errorf("Versions[neoforge] = %q, want %q", pack.Versions["neoforge"], "21.1.213")
	}
}
