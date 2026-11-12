package matching

import (
	"reflect"
	"strings"
	"testing"

	"my-go-exchange/internal/constant"
)

func TestEventEnvelopeRoundTrip(t *testing.T) {
	tests := []Event{
		&OrderAccepted{
			Seq:           1,
			OrderID:       2,
			AccountID:     3,
			ClientOrderID: "client-2",
			Symbol:        "TEST/QUOTE",
			Price:         100,
			Amount:        4,
			OrderSide:     constant.Buy,
			PositionSide:  constant.Long,
			Type:          constant.Limit,
			Leverage:      5,
			// TIF:       constant.GTC,
		},
		&OrderCanceled{Seq: 2, OrderID: 2, Remaining: 1, Reason: "user_canceled"},
		&OrderRejected{
			Seq:           3,
			OrderID:       2,
			AccountID:     3,
			ClientOrderID: "client-2",
			Reason:        "invalid",
		},
		&TradeEvent{
			Seq:            4,
			TradeID:        1,
			Price:          100,
			Amount:         3,
			TakerSide:      constant.Buy,
			TakerOrderID:   2,
			MakerOrderID:   1,
			TakerAccountID: 3,
			MakerAccountID: 4,
			MakerState:     constant.StatePartial,
			MakerRemaining: 1,
			TakerState:     constant.StateFilled,
			TakerRemaining: 0,
		},
	}

	for _, want := range tests {
		data, err := MarshalEvent(want)
		if err != nil {
			t.Fatalf("MarshalEvent(%T): %v", want, err)
		}

		got, err := UnmarshalEvent(data)
		if err != nil {
			t.Fatalf("UnmarshalEvent(%T): %v", want, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("round trip %T = %#v, want %#v", want, got, want)
		}
		if got.Kind() != want.Kind() || got.Sequence() != want.Sequence() {
			t.Fatalf("event metadata = (%q,%d), want (%q,%d)",
				got.Kind(), got.Sequence(), want.Kind(), want.Sequence())
		}
	}
}

func TestEventEnvelopeErrors(t *testing.T) {
	if _, err := MarshalEvent(nil); err == nil {
		t.Fatal("MarshalEvent(nil) should fail")
	}
	if _, err := MarshalEvent(&TradeEvent{Seq: 1, TradeID: 1}); err != nil {
		t.Fatalf("MarshalEvent supported event: %v", err)
	}

	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "invalid JSON", data: `{`, want: "decode event envelope"},
		{name: "missing type", data: `{"payload":{}}`, want: "missing type"},
		{name: "unknown type", data: `{"type":"order_expired","payload":{}}`, want: "unknown type"},
		{name: "missing payload", data: `{"type":"trade"}`, want: "missing payload"},
		{
			name: "unknown payload field",
			data: `{"type":"trade","payload":{"seq":1,"extra":true}}`,
			want: "decode trade payload",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := UnmarshalEvent([]byte(test.data))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}
