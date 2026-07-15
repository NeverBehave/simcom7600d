# SIMCom reference documents

The daemon's SMS, voice-call, supplementary-service, USB-mode, and raw PCM
implementations are based on the official **SIM7500/SIM7600 Series AT Command
Manual v3.00**, released on 2021-11-18.

SIMCom marks the manual as proprietary and does not grant redistribution
rights. The repository therefore tracks this reference note and the extraction
tool, but not a downloaded PDF or a full generated copy. Obtain the current
manual from [SIMCom's document download page][simcom-download] and keep it
locally at:

```text
docs/reference/simcom/SIM7500_SIM7600_AT_Command_Manual_v3.00.pdf
```

For fast local searching, generate a Markdown extract with:

```sh
python3 -m pip install pypdf
python3 tools/extract_simcom_manual.py
```

The output is written beside the PDF as
`SIM7500_SIM7600_AT_Command_Manual_v3.00.generated.md`. Both files are ignored
by Git. The extractor also accepts explicit input and output paths.

Frequently consulted areas include basic call control (`ATD`, `ATA`,
`AT+CHUP`, and `AT+CLCC`), SMS storage and transfer (`AT+CPMS`, `AT+CMGL`,
`AT+CMGR`, `AT+CMGS`, and `AT+CMGD`), call forwarding (`AT+CCFC`), USB product
mode (`AT+CUSBPIDSWITCH`), and SIMCom PCM/audio commands.

[simcom-download]: https://www.simcom.com/download/list-863-en.html
