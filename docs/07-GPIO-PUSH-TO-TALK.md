# GPIO Push-to-Talk Plan

This is a planned feature, not part of the current working voice demo.

## Pin names

The Luckfox 40-pin header gives the same physical pad several names:

| Name | Meaning |
|---|---|
| Physical pin 5 | Position on the header; third contact from the right in the photographed orientation |
| `I2C2_SCL_M0` | Default I2C clock function |
| `GPIO1_A0_d` | SoC GPIO identifier |
| GPIO32 | Legacy Linux GPIO number: bank 1 × 32 + A0 |

Physical pin 3 is `I2C2_SDA_M0` / `GPIO1_A1_d` / GPIO33. Physical pin 9 is
GND. SCL is physical pin 5, not physical pin 3.

`GPIO1_A0_d` is a hardware identifier, not a Python or Go keyword:

- `GPIO`: general-purpose input/output;
- `1`: GPIO bank 1;
- `A`: group A;
- `0`: bit 0;
- `_d`: part of Luckfox's pin label; pinctrl still explicitly selects the mux
  and pull configuration.

## Safe button wiring

```text
physical pin 5 / GPIO32 ---- normally-open button ---- physical pin 9 / GND
```

Configure pin 5 as a GPIO input with a pull-up:

```text
released -> HIGH (1)
pressed  -> LOW  (0)
```

“Short SCL to GND” means the button temporarily connects the repurposed pin to
GND. Never connect it permanently, never connect it to 5 V, and do not press
it while the pad is still actively configured as I2C SCL.

## Required software work

1. Disable the conflicting I2C SCL function.
2. Select GPIO mode for bank 1, port A, pin 0.
3. Configure input and pull-up through Luckfox pinctrl/device tree.
4. Detect the HIGH-to-LOW edge with 30–50 ms debounce.
5. Send a button event to the Python agent or gate microphone publishing.
6. Define press/release behavior, such as hold-to-talk or press-to-arm.

Conceptual pinctrl identifiers are `1`, `RK_PA0`, `RK_FUNC_GPIO`, and
`pcfg_pull_up`. The exact device-tree node must match this board image and
firmware; Linux sysfs direction commands alone do not configure the pinmux or
pull resistor.

## Planned workflow

```text
press button -> GPIO32 LOW -> start/arm listening -> speak
release button -> finish turn -> Sarvam STT -> DeepSeek -> Sarvam TTS
```

Until the Go client and cloud agent implement that event, continue using the
`Hey Siri` wake phrase.

Official references:

- <https://wiki.luckfox.com/Luckfox-Pico-Pi/Pinout/Pin-Definition/>
- <https://wiki.luckfox.com/Luckfox-Pico-RV1106/Peripherals/Luckfox-config/>
