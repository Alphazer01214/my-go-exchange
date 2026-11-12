package matching

import (
	"reflect"
	"strings"
	"testing"

	"my-go-exchange/internal/constant"
)

func TestCommandEnvelopeRoundTrip(t *testing.T) {
	tests := []Command{
		&PlaceOrder{
			AccountID:     7,
			ClientOrderID: "client-7",
			Price:         123,
			Amount:        9,
			Leverage:      10,
			OrderSide:     constant.Sell,
			PositionSide:  constant.Short,
			Type:          constant.Limit,
			// TIF:       constant.IOC,
		},
		&CancelOrder{OrderID: 42},
	}

	for _, want := range tests {
		data, err := MarshalCommand(want)
		if err != nil {
			t.Fatalf("MarshalCommand(%T): %v", want, err)
		}

		got, err := UnmarshalCommand(data)
		if err != nil {
			t.Fatalf("UnmarshalCommand(%T): %v", want, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("round trip %T = %#v, want %#v", want, got, want)
		}
		if got.Kind() != want.Kind() {
			t.Fatalf("Kind() = %q, want %q", got.Kind(), want.Kind())
		}
	}
}

func TestCommandEnvelopeErrors(t *testing.T) {
	if _, err := MarshalCommand(nil); err == nil {
		t.Fatal("MarshalCommand(nil) should fail")
	}
	if _, err := MarshalCommand(&PlaceOrder{}); err != nil {
		t.Fatalf("MarshalCommand supported command: %v", err)
	}

	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "invalid JSON", data: `{`, want: "decode command envelope"},
		{name: "missing type", data: `{"payload":{}}`, want: "missing type"},
		{name: "unknown type", data: `{"type":"replace_order","payload":{}}`, want: "unknown type"},
		{name: "missing payload", data: `{"type":"place_order"}`, want: "missing payload"},
		{
			name: "missing place field",
			data: `{"type":"place_order","payload":{"account_id":1,"price":2,"side":"buy","type":"limit"}}`,
			want: `missing field "amount"`,
		},
		{
			name: "missing cancel field",
			data: `{"type":"cancel_order","payload":{}}`,
			want: `missing field "order_id"`,
		},
		{
			name: "trailing JSON",
			data: `{"type":"cancel_order","payload":{"order_id":1}} {}`,
			want: "multiple JSON values",
		},
		{
			name: "unknown payload field",
			data: `{"type":"cancel_order","payload":{"order_id":1,"extra":true}}`,
			want: "decode cancel_order payload",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := UnmarshalCommand([]byte(test.data))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}
