// Copyright (c) 2023 Cloudnatively Services Pvt Ltd
//
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	vus          = "10"
	duration     = "5m"
	schema_count = "20"
)

func TestSmokeListLogStream(t *testing.T) {
	req, err := NewGlob.Client.NewRequest("GET", "logstream", nil)
	require.NoErrorf(t, err, "Request failed: %s", err)

	response, err := NewGlob.Client.Do(req)
	require.NoErrorf(t, err, "Request failed: %s", err)

	body := readAsString(response.Body)
	require.Equalf(t, 200, response.StatusCode, "Server returned http code: %s and response: %s", response.Status)
	res, err := readJsonBody[[]string](bytes.NewBufferString(body))
	if err != nil {
		for _, stream := range res {
			if stream == NewGlob.Stream {
				DeleteStream(t, NewGlob.Client, NewGlob.Stream)
			}
		}
	}
}

func TestSmokeCreateStream(t *testing.T) {
	CreateStream(t, NewGlob.Client, NewGlob.Stream)
}

func TestSmokeIngestEventsToStream(t *testing.T) {
	cmd := exec.Command("flog", "-f", "json", "-n", "50")
	var out strings.Builder
	cmd.Stdout = &out
	err := cmd.Run()
	require.NoErrorf(t, err, "Failed to run flog: %s", err)

	for _, obj := range strings.SplitN(out.String(), "\n", 50) {
		var payload strings.Builder
		payload.WriteRune('[')
		payload.WriteString(obj)
		payload.WriteRune(']')

		req, _ := NewGlob.Client.NewRequest("POST", "ingest", bytes.NewBufferString(payload.String()))
		req.Header.Add("X-P-Stream", NewGlob.Stream)
		response, err := NewGlob.Client.Do(req)
		require.NoErrorf(t, err, "Request failed: %s", err)
		require.Equalf(t, 200, response.StatusCode, "Server returned http code: %s resp %s", response.Status, readAsString(response.Body))
	}

	QueryLogStreamCount(t, NewGlob.Client, NewGlob.Stream, 50)
	AssertStreamSchema(t, NewGlob.Client, NewGlob.Stream, FlogJsonSchema)
	DeleteStream(t, NewGlob.Client, NewGlob.Stream)
	Sleep()
}

func TestSmokeModulesAPI(t *testing.T) {
	CreateStream(t, NewGlob.Client, NewGlob.Stream)
	test_module_registration_flow(t)
	DeleteStream(t, NewGlob.Client, NewGlob.Stream)
}

func TestSmokeLoadWithK6Stream(t *testing.T) {
	CreateStream(t, NewGlob.Client, NewGlob.Stream)
	cmd := exec.Command("k6",
		"run",
		"-e", fmt.Sprintf("P_URL=%s", NewGlob.Url.String()),
		"-e", fmt.Sprintf("P_USERNAME=%s", NewGlob.Username),
		"-e", fmt.Sprintf("P_PASSWORD=%s", NewGlob.Password),
		"-e", fmt.Sprintf("P_STREAM=%s", NewGlob.Stream),
		"./scripts/smoke.js")

	cmd.Run()
	cmd.Output()
	QueryLogStreamCount(t, NewGlob.Client, NewGlob.Stream, 60000)
	AssertStreamSchema(t, NewGlob.Client, NewGlob.Stream, SchemaBody)
}

func TestSmokeSetAlert(t *testing.T) {
	req, _ := NewGlob.Client.NewRequest("PUT", "logstream/"+NewGlob.Stream+"/alert", strings.NewReader(AlertBody))
	response, err := NewGlob.Client.Do(req)
	require.NoErrorf(t, err, "Request failed: %s", err)
	require.Equalf(t, 200, response.StatusCode, "Server returned http code: %s and response: %s", response.Status, readAsString(response.Body))
}

func TestSmokeGetAlert(t *testing.T) {
	req, _ := NewGlob.Client.NewRequest("GET", "logstream/"+NewGlob.Stream+"/alert", nil)
	response, err := NewGlob.Client.Do(req)
	require.NoErrorf(t, err, "Request failed: %s", err)
	body := readAsString(response.Body)
	require.Equalf(t, 200, response.StatusCode, "Server returned http code: %s and response: %s", response.Status, body)
	require.JSONEq(t, AlertBody, body, "Get alert response doesn't match with Alert config returned")
}

func TestSmokeSetRetention(t *testing.T) {
	req, _ := NewGlob.Client.NewRequest("PUT", "logstream/"+NewGlob.Stream+"/retention", strings.NewReader(RetentionBody))
	response, err := NewGlob.Client.Do(req)
	require.NoErrorf(t, err, "Request failed: %s", err)
	require.Equalf(t, 200, response.StatusCode, "Server returned http code: %s and response: %s", response.Status, readAsString(response.Body))
}

func TestSmokeGetRetention(t *testing.T) {
	req, _ := NewGlob.Client.NewRequest("GET", "logstream/"+NewGlob.Stream+"/retention", nil)
	response, err := NewGlob.Client.Do(req)
	require.NoErrorf(t, err, "Request failed: %s", err)
	body := readAsString(response.Body)
	require.Equalf(t, 200, response.StatusCode, "Server returned http code: %s and response: %s", response.Status, body)
	require.JSONEq(t, RetentionBody, body, "Get retention response doesn't match with retention config returned")
}

func TestSmokeRbacBasic(t *testing.T) {
	SetRole(t, NewGlob.Client, "dummy", dummyRole)
	AssertRole(t, NewGlob.Client, "dummy", dummyRole)
	CreateUserWithRole(t, NewGlob.Client, "dummy", []string{"dummy"})
	userClient := NewGlob.Client
	userClient.Username = "dummy"
	userClient.Password = RegenPassword(t, NewGlob.Client, "dummy")
	checkAPIAccess(t, userClient, NewGlob.Stream, "editor")
	DeleteUser(t, NewGlob.Client, "dummy")
	DeleteRole(t, NewGlob.Client, "dummy")
}

func TestSmokeRoles(t *testing.T) {
	cases := []struct {
		roleName string
		body     string
	}{
		{
			roleName: "editor",
			body:     RoleEditor,
		},
		{
			roleName: "reader",
			body:     RoleReader(NewGlob.Stream),
		},
		{
			roleName: "writer",
			body:     RoleWriter(NewGlob.Stream),
		},
		{
			roleName: "ingest",
			body:     RoleIngestor(NewGlob.Stream),
		},
	}

	for _, tc := range cases {
		t.Run(tc.roleName, func(t *testing.T) {
			SetRole(t, NewGlob.Client, tc.roleName, tc.body)
			AssertRole(t, NewGlob.Client, tc.roleName, tc.body)
			username := tc.roleName + "_user"
			password := CreateUserWithRole(t, NewGlob.Client, username, []string{tc.roleName})

			userClient := NewGlob.Client
			userClient.Username = username
			userClient.Password = password
			checkAPIAccess(t, userClient, NewGlob.Stream, tc.roleName)
			DeleteUser(t, NewGlob.Client, username)
			DeleteRole(t, NewGlob.Client, tc.roleName)
		})
	}
}

func TestLoadStreamBatchWithK6(t *testing.T) {
	if NewGlob.Mode == "load" {
		cmd := exec.Command("k6",
			"run",
			"-e", fmt.Sprintf("P_URL=%s", NewGlob.Url.String()),
			"-e", fmt.Sprintf("P_USERNAME=%s", NewGlob.Username),
			"-e", fmt.Sprintf("P_PASSWORD=%s", NewGlob.Password),
			"-e", fmt.Sprintf("P_STREAM=%s", NewGlob.Stream),
			"-e", fmt.Sprintf("P_SCHEMA_COUNT=%s", schema_count),
			"./scripts/load_batch_events.js",
			"--vus=", vus,
			"--duration=", duration)

		cmd.Run()
		op, err := cmd.Output()
		if err != nil {
			t.Log(err)
		}
		t.Log(string(op))
	}
}

func TestLoadStreamNoBatchWithK6(t *testing.T) {
	if NewGlob.Mode == "load" {
		cmd := exec.Command("k6",
			"run",
			"-e", fmt.Sprintf("P_URL=%s", NewGlob.Url.String()),
			"-e", fmt.Sprintf("P_USERNAME=%s", NewGlob.Username),
			"-e", fmt.Sprintf("P_PASSWORD=%s", NewGlob.Password),
			"-e", fmt.Sprintf("P_STREAM=%s", NewGlob.Stream),
			"-e", fmt.Sprintf("P_SCHEMA_COUNT=%s", schema_count),
			"./scripts/load_single_events.js",
			"--vus=", vus,
			"--duration=", duration)

		cmd.Run()
		op, err := cmd.Output()
		if err != nil {
			t.Log(err)
		}
		t.Log(string(op))
	}
}

func TestDeleteStream(t *testing.T) {
	DeleteStream(t, NewGlob.Client, NewGlob.Stream)
}
