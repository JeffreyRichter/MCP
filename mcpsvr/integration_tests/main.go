// This program runs integration tests for the MCP service using Azurite to emulate Azure Storage.
// It starts the necessary services with docker-compose, runs the tests, and cleans up afterwards.
// It doesn't follow `go test` conventions because it has complex setup and teardown requirements,
// and to avoid making every `go test ./...` slow and complicated.
//
// Define new test cases as TestSuite methods (see test_suite.go) with value receivers.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
	"time"
)

var ctx = context.Background()

func main() {
	tests := discoverTests()
	if len(tests) == 0 {
		fmt.Println("No tests found")
		os.Exit(1)
	}

	if err := startServices(ctx); isError(err) {
		fmt.Printf("Failed to start services: %v", err)
		os.Exit(1)
	}
	defer cleanup()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := run(tests); isError(err) {
			fmt.Println(err)
		}
		sigChan <- syscall.SIGTERM
	}()

	<-sigChan
	fmt.Println("Shutting down...")
}

type testInfo struct {
	Name string
	Run  func() error
}

func run(tests []testInfo) error {
	fmt.Println("Waiting for services to be ready...")
	// TODO: add health check so we know service is ready when `docker-compose up` exits
	time.Sleep(2 * time.Second)

	var failed []string
	for i, test := range tests {
		fmt.Printf("\n=== %s (%d/%d) ===\n", test.Name, i+1, len(tests))
		if err := test.Run(); isError(err) {
			failed = append(failed, test.Name)
			fmt.Printf("\t%s\n=== %s failed ===\n", err, test.Name)
		} else {
			fmt.Printf("=== %s passed ===\n", test.Name)
		}
	}
	if len(failed) > 0 {
		return errors.New("Failed tests:\n\t" + strings.Join(failed, "\n\t"))
	}
	fmt.Println("\nAll tests passed")
	return nil
}

func discoverTests() []testInfo {
	var tests []testInfo

	suite := TestSuite{}
	suiteType := reflect.TypeOf(suite)
	suiteValue := reflect.ValueOf(suite)

	for i := 0; i < suiteType.NumMethod(); i++ {
		method := suiteType.Method(i)
		methodValue := suiteValue.Method(i)

		// Check if method name starts with "Test" and has correct signature
		if strings.HasPrefix(method.Name, "Test") &&
			method.Type.NumIn() == 1 && // receiver only
			method.Type.NumOut() == 1 &&
			method.Type.Out(0) == reflect.TypeOf((*error)(nil)).Elem() {

			// this closure calls the method
			testFunc := func(mv reflect.Value) func() error {
				return func() error {
					results := mv.Call(nil)
					if len(results) > 0 && !results[0].IsNil() {
						return results[0].Interface().(error)
					}
					return nil
				}
			}(methodValue)

			tests = append(tests, testInfo{
				Name: method.Name,
				Run:  testFunc,
			})
		}
	}

	return tests
}

func startServices(ctx context.Context) error {
	fmt.Println("Starting containers...")

	cmd := exec.CommandContext(ctx, "docker", "compose", "up", "--build", "--wait", "--wait-timeout", "60")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func cleanup() {
	fmt.Println("=========== mcpsvr log ===========")
	logCmd := exec.Command("docker", "logs", "mcpsvr")
	logCmd.Stdout = os.Stdout
	logCmd.Stderr = os.Stderr
	logCmd.WaitDelay = time.Second
	if err := logCmd.Run(); isError(err) {
		fmt.Println(err)
	}

	fmt.Println("=========== cleaning up ===========")
	downCmd := exec.Command("docker", "compose", "down", "-v")
	downCmd.Stdout = os.Stdout
	downCmd.Stderr = os.Stderr
	logCmd.WaitDelay = time.Second
	if err := downCmd.Run(); isError(err) {
		fmt.Println(err)
	}
}
func isError(err error) bool { return err != nil }
