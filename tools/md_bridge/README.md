# md_bridge

Python CTP market-data bridge used by Go `md-ingest`.

## Dependency

```bash
pip install openctp-ctp
```

- SimNow (default): `tcp://182.254.243.31:40011` — works with `openctp-ctp`
- openctp TTS: `tcp://trading.openctp.cn:30011` — needs TTS-compatible binding; stock `openctp-ctp` returns disconnect code `4097`

## Run

```bash
python md_bridge.py --front tcp://182.254.243.31:40011 --symbols cu2611,al2611,rb2701,au2612
```

Stdout is NDJSON (`type=tick` / `type=status`).
