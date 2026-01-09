package sshtunnel_test

import (
	"reflect"
	"testing"

	"github.com/jamesgdiaz/sshtunnel"
)

func TestCreateEndpoint(t *testing.T) {
	// these are test cases for which we expect no error to occur when
	// constructing endpoints i.e. they should be correct
	testCases := []struct {
		input            string
		expectedEndpoint *sshtunnel.Endpoint
	}{
		{
			"localhost:9000",
			&sshtunnel.Endpoint{
				Host: "localhost",
				Port: 9000,
				User: "",
			},
		},
		{
			"ec2-user@jumpbox.us-east-1.mydomain.com",
			&sshtunnel.Endpoint{
				Host: "jumpbox.us-east-1.mydomain.com",
				Port: 0,
				User: "ec2-user",
			},
		},
		{
			"dqrsdfdssdfx.us-east-1.redshift.amazonaws.com:5439",
			&sshtunnel.Endpoint{
				Host: "dqrsdfdssdfx.us-east-1.redshift.amazonaws.com",
				Port: 5439,
				User: "",
			},
		},
		{
			"admin@1.2.3.4:22", // IPv4 address
			&sshtunnel.Endpoint{
				Host: "1.2.3.4",
				Port: 22,
				User: "admin",
			},
		},
		{
			"admin@[2001:db8:1::ab9:C0A8:102]:22", // IPv6 address
			&sshtunnel.Endpoint{
				Host: "2001:db8:1::ab9:C0A8:102",
				Port: 22,
				User: "admin",
			},
		},
	}
	for i, tc := range testCases {
		got, err := sshtunnel.NewEndpoint(tc.input)
		if err != nil {
			t.Errorf("unexpected error for correct input '%s': %v",
				tc.input, err)
		}
		if !reflect.DeepEqual(got, tc.expectedEndpoint) {
			t.Errorf("For test case %d, expected: %+v, got: %+v",
				i, *tc.expectedEndpoint, *got)
		}
	}
}
