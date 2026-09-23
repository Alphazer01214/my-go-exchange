"""CTP market-data bridge (Python).

Uses openctp_ctp (official CTPAPI binding) against SimNow or TTS fronts.
Emits newline-delimited JSON ticks on stdout for the Go md-ingest consumer.

Usage:
  python md_bridge.py --front tcp://182.254.243.31:40011 --symbols cu2611,al2611
"""

from __future__ import annotations

import argparse
import json
import sys
import time
from datetime import datetime

from openctp_ctp import thostmduserapi as mdapi

DEFAULT_FRONT = "tcp://182.254.243.31:40011"
DEFAULT_BROKER = "9999"
DEFAULT_SYMBOLS = ["cu2611", "al2611", "rb2701", "au2612"]

EXCHANGE_PREFIX = {
    "SHFE": {"cu", "au", "ag", "al", "zn", "rb", "hc", "ru", "bu", "ni", "sn", "ss", "pb", "wr", "sp"},
    "DCE": {"m", "y", "c", "cs", "a", "b", "p", "l", "pp", "v", "eb", "eg", "pg", "i", "j", "jm", "rr", "lh", "fb"},
    "CZCE": {"ma", "ta", "fg", "pf", "rm", "sr", "cf", "cy", "oi", "wh", "pm", "ri", "rs", "sf", "sm", "ur", "sa"},
    "CFFEX": {"if", "ih", "ic", "im", "tf", "ts", "t", "tl"},
    "INE": {"sc", "bc", "lu", "nr", "ec"},
    "GFEX": {"si", "lc", "ps"},
}


def guess_exchange(instrument_id: str) -> str:
    prefix = "".join(ch for ch in instrument_id if ch.isalpha()).lower()
    for exchange, prefixes in EXCHANGE_PREFIX.items():
        if prefix in prefixes:
            return exchange
    return "UNKNOWN"


def emit(obj: dict) -> None:
    sys.stdout.write(json.dumps(obj, ensure_ascii=False) + "\n")
    sys.stdout.flush()


class MdBridge(mdapi.CThostFtdcMdSpi):
    def __init__(self, front: str, symbols: list[str], broker: str, user: str, password: str):
        super().__init__()
        self.front = front
        self.symbols = symbols
        self.broker = broker
        self.user = user
        self.password = password
        self.api = None
        self.connected = False
        self.subscribed: set[str] = set()
        self.tick_count = 0

    def start(self) -> None:
        self.api = mdapi.CThostFtdcMdApi.CreateFtdcMdApi()
        self.api.RegisterSpi(self)
        self.api.RegisterFront(self.front)
        emit({"type": "status", "event": "init", "front": self.front, "symbols": self.symbols})
        self.api.Init()

    def OnFrontConnected(self) -> None:
        self.connected = True
        emit({"type": "status", "event": "front_connected", "front": self.front})
        req = mdapi.CThostFtdcReqUserLoginField()
        if self.broker:
            req.BrokerID = self.broker
        if self.user:
            req.UserID = self.user
        if self.password:
            req.Password = self.password
        self.api.ReqUserLogin(req, 1)

    def OnFrontDisconnected(self, reason: int) -> None:
        self.connected = False
        emit({"type": "status", "event": "front_disconnected", "reason": reason})

    def OnRspUserLogin(self, login, info, req_id, is_last) -> None:
        if info is not None and info.ErrorID != 0:
            emit({"type": "status", "event": "login_failed", "error_id": info.ErrorID, "error_msg": info.ErrorMsg})
            return
        trading_day = login.TradingDay if login else ""
        emit({"type": "status", "event": "login_ok", "trading_day": trading_day})
        ids = [s.encode("utf-8") for s in self.symbols]
        self.api.SubscribeMarketData(ids, len(ids))

    def OnRspSubMarketData(self, instrument, info, req_id, is_last) -> None:
        instrument_id = instrument.InstrumentID if instrument else ""
        if info is not None and info.ErrorID != 0:
            emit({
                "type": "status",
                "event": "subscribe_failed",
                "instrument_id": instrument_id,
                "error_id": info.ErrorID,
                "error_msg": info.ErrorMsg,
            })
            return
        self.subscribed.add(instrument_id)
        emit({"type": "status", "event": "subscribe_ok", "instrument_id": instrument_id})

    def OnRspError(self, info, req_id, is_last) -> None:
        if info is not None:
            emit({"type": "status", "event": "error", "error_id": info.ErrorID, "error_msg": info.ErrorMsg})

    def OnRtnDepthMarketData(self, d) -> None:
        # CTP double "empty" sentinel is ~1.7976931348623157e+308
        def price(v: float) -> float | None:
            if v is None:
                return None
            if abs(v) > 1e15:
                return None
            return float(v)

        instrument_id = d.InstrumentID
        self.tick_count += 1
        now = datetime.now()
        update_time = d.UpdateTime  # HH:MM:SS
        payload = {
            "type": "tick",
            "instrument_id": instrument_id,
            "exchange": guess_exchange(instrument_id),
            "price": price(d.LastPrice),
            "volume": int(d.Volume or 0),
            "bid": price(d.BidPrice1),
            "bid_volume": int(d.BidVolume1 or 0),
            "ask": price(d.AskPrice1),
            "ask_volume": int(d.AskVolume1 or 0),
            "open_interest": float(d.OpenInterest or 0),
            "turnover": float(d.Turnover or 0),
            "open": price(d.OpenPrice),
            "high": price(d.HighestPrice),
            "low": price(d.LowestPrice),
            "pre_close": price(d.PreClosePrice),
            "pre_settlement": price(d.PreSettlementPrice),
            "upper_limit": price(d.UpperLimitPrice),
            "lower_limit": price(d.LowerLimitPrice),
            "update_time": update_time,
            "update_millisec": int(d.UpdateMillisec or 0),
            "trading_day": d.TradingDay,
            "action_day": getattr(d, "ActionDay", "") or "",
            "timestamp": time.time(),
            "source": "ctp-bridge",
            "source_mode": "ctp",
        }
        emit(payload)


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="CTP MD bridge -> NDJSON on stdout")
    p.add_argument("--front", default=DEFAULT_FRONT)
    p.add_argument("--symbols", default=",".join(DEFAULT_SYMBOLS))
    p.add_argument("--broker", default=DEFAULT_BROKER)
    p.add_argument("--user", default="")
    p.add_argument("--password", default="")
    return p.parse_args()


def main() -> int:
    args = parse_args()
    symbols = [s.strip() for s in args.symbols.split(",") if s.strip()]
    if not symbols:
        emit({"type": "status", "event": "fatal", "error_msg": "no symbols"})
        return 2

    bridge = MdBridge(args.front, symbols, args.broker, args.user, args.password)
    bridge.start()
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        emit({"type": "status", "event": "shutdown", "tick_count": bridge.tick_count})
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
