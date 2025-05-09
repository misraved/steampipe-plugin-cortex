package cortex

import (
	"net/http"
	"testing"

	_ "unsafe"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"github.com/turbot/steampipe-plugin-sdk/v5/grpc/proto"
	"gopkg.in/yaml.v3"
)

func prepareTeamResponse(t *testing.T, teams []CortexTeamElement) []byte {
	t.Helper()
	response := CortexTeamResponse{
		Teams: teams,
	}
	responseBytes, err := yaml.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}
	return responseBytes
}

func TestTableCortexTeam(t *testing.T) {
	g := NewWithT(t)
	table := tableCortexTeam()

	// Check basic table properties.
	g.Expect(table).ToNot(BeNil())
	g.Expect(table.Name).To(Equal("cortex_team"))
	g.Expect(table.Description).To(Equal("Cortex list teams api."))

	// Check list configuration.
	g.Expect(table.List).ToNot(BeNil())
	g.Expect(table.List.Hydrate).ToNot(BeNil())

	// Define expected columns.
	expectedColumns := []struct {
		Name string
		Type proto.ColumnType
	}{
		{"name", proto.ColumnType_STRING},
		{"tag", proto.ColumnType_STRING},
		{"parents", proto.ColumnType_JSON},
		{"children", proto.ColumnType_JSON},
		{"metadata", proto.ColumnType_JSON},
		{"links", proto.ColumnType_JSON},
		{"archived", proto.ColumnType_BOOL},
		{"slack_channels", proto.ColumnType_JSON},
		{"members", proto.ColumnType_JSON},
	}

	// Check that the table has the expected columns.
	g.Expect(table.Columns).To(HaveLen(len(expectedColumns)))
	for i, exp := range expectedColumns {
		g.Expect(table.Columns[i].Name).To(Equal(exp.Name))
		g.Expect(table.Columns[i].Type).To(Equal(exp.Type))
	}
}

func TestListTeamsSinglePage(t *testing.T) {
	g := NewWithT(t)
	gh := ghttp.NewGHTTPWithGomega(g)

	responseBytes := prepareTeamResponse(t, []CortexTeamElement{{Tag: "team1"}})

	ctx, server, client := setupTestServerAndClient(t,
		ghttp.CombineHandlers(
			gh.VerifyRequest("GET", "/api/v1/teams"),
			gh.VerifyHeaderKV("Authorization", "Bearer fake_api_key"),
			gh.RespondWith(http.StatusOK, responseBytes, nil),
		),
	)
	defer server.Close()

	writer := NewSliceWriter[CortexTeamElement](100)

	relationships := map[string]Relationships{
		"team1": {
			Children: []string{"child1"},
			Parents:  []string{"parent1"},
		},
	}

	err := listTeams(ctx, client, writer, relationships)
	g.Expect(err).To(BeNil())

	g.Expect(writer.Items).To(HaveLen(1))
	g.Expect(writer.Items[0].Tag).To(Equal("team1"))
	g.Expect(writer.Items[0].Children).To(HaveLen(1))
	g.Expect(writer.Items[0].Children[0]).To(Equal("child1"))
	g.Expect(writer.Items[0].Parents).To(HaveLen(1))
	g.Expect(writer.Items[0].Parents[0]).To(Equal("parent1"))
}

func TestListTeamsError(t *testing.T) {
	g := NewWithT(t)
	gh := ghttp.NewGHTTPWithGomega(g)

	ctx, server, client := setupTestServerAndClient(t,
		ghttp.CombineHandlers(
			gh.VerifyRequest("GET", "/api/v1/teams"),
			gh.VerifyHeaderKV("Authorization", "Bearer fake_api_key"),
			gh.RespondWith(http.StatusInternalServerError, "{\"details\": \"fake error on teams\"}", nil),
		),
	)
	defer server.Close()

	writer := NewSliceWriter[CortexTeamElement](100)

	relationships := map[string]Relationships{}

	err := listTeams(ctx, client, writer, relationships)
	g.Expect(err).ToNot(BeNil())
	g.Expect(err.Error()).To(Equal("error from cortex API 500 Internal Server Error: {\"details\": \"fake error on teams\"}"))
}

func prepareRelationshipsResponse(t *testing.T, edges []CortexRelationshipsEdge) []byte {
	t.Helper()
	response := CortexRelationshipsResponse{
		Edges: edges,
	}
	responseBytes, err := yaml.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal relationships response: %v", err)
	}
	return responseBytes
}

func TestGetTeamRelationshipsSuccess(t *testing.T) {
	g := NewWithT(t)
	gh := ghttp.NewGHTTPWithGomega(g)

	// Prepare a relationships response with one edge.
	// For an edge with Child "child1" and Parent "parent1", as per code,
	// relationships["child1"].Parents should contain "parent1"
	// and relationships["parent1"].Children should contain "parent1"
	responseBytes := prepareRelationshipsResponse(t, []CortexRelationshipsEdge{
		{Child: "child1", Parent: "parent1"},
	})

	ctx, server, client := setupTestServerAndClient(t,
		ghttp.CombineHandlers(
			gh.VerifyRequest("GET", "/api/v1/teams/relationships"),
			gh.VerifyHeaderKV("Authorization", "Bearer fake_api_key"),
			gh.RespondWith(http.StatusOK, responseBytes, nil),
		),
	)
	defer server.Close()

	relationships, err := getTeamRelationships(ctx, client)
	g.Expect(err).To(BeNil())
	g.Expect(relationships).To(HaveKey("child1"))
	g.Expect(relationships["child1"].Parents).To(ContainElement("parent1"))
	g.Expect(relationships).To(HaveKey("parent1"))
	g.Expect(relationships["parent1"].Children).To(ContainElement("child1"))
}

func TestGetTeamRelationshipsHTTPError(t *testing.T) {
	g := NewWithT(t)
	gh := ghttp.NewGHTTPWithGomega(g)
	ctx, server, client := setupTestServerAndClient(t,
		ghttp.CombineHandlers(
			gh.VerifyRequest("GET", "/api/v1/teams/relationships"),
			gh.VerifyHeaderKV("Authorization", "Bearer fake_api_key"),
			gh.RespondWith(http.StatusInternalServerError, "{\"details\": \"fake error on relationships\"}", nil),
		),
	)
	defer server.Close()

	relationships, err := getTeamRelationships(ctx, client)
	g.Expect(err).ToNot(BeNil())
	g.Expect(relationships).To(BeNil())
	g.Expect(err.Error()).To(Equal("error from cortex API 500 Internal Server Error: {\"details\": \"fake error on relationships\"}"))
}

func TestGetTeamRelationshipsInvalidYAML(t *testing.T) {
	g := NewWithT(t)
	gh := ghttp.NewGHTTPWithGomega(g)
	ctx, server, client := setupTestServerAndClient(t,
		ghttp.CombineHandlers(
			gh.VerifyRequest("GET", "/api/v1/teams/relationships"),
			gh.VerifyHeaderKV("Authorization", "Bearer fake_api_key"),
			gh.RespondWith(http.StatusOK, "invalid: yaml: : data", nil),
		),
	)
	defer server.Close()

	relationships, err := getTeamRelationships(ctx, client)
	g.Expect(err).ToNot(BeNil())
	g.Expect(relationships).To(BeNil())
}
