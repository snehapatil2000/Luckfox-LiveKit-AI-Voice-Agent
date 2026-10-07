from livekit.agents import llm

import agent as agent_module
from agent import Assistant


def _tool(assistant: Assistant, name: str) -> llm.FunctionTool:
    tools = [
        tool
        for tool in assistant.tools
        if isinstance(tool, llm.FunctionTool) and tool.info.name == name
    ]
    assert len(tools) == 1
    return tools[0]


def test_all_function_tools_are_registered() -> None:
    tool_names = {
        tool.info.name
        for tool in Assistant().tools
        if isinstance(tool, llm.FunctionTool)
    }

    assert tool_names == {
        "calculator",
        "get_current_time",
        "get_weather",
        "search_database",
    }


async def test_calculator_adds_numbers() -> None:
    tool = _tool(Assistant(), "calculator")

    result = await tool(
        operation="add",
        first_number=25,
        second_number=17,
    )

    assert result == "The calculator result is 42."


async def test_calculator_handles_division_by_zero() -> None:
    tool = _tool(Assistant(), "calculator")

    result = await tool(
        operation="divide",
        first_number=10,
        second_number=0,
    )

    assert result == "Division by zero is not allowed."


async def test_get_current_time_returns_requested_location_time(monkeypatch) -> None:
    async def fake_fetch_json(url, parameters):
        if url == agent_module.OPEN_METEO_GEOCODING_URL:
            return {
                "results": [
                    {
                        "name": "Tokyo",
                        "country": "Japan",
                        "latitude": 35.68,
                        "longitude": 139.69,
                    }
                ]
            }
        return {
            "utc_offset_seconds": 32400,
            "timezone_abbreviation": "JST",
        }

    monkeypatch.setattr(agent_module, "_fetch_json", fake_fetch_json)
    tool = _tool(Assistant(), "get_current_time")

    result = await tool(location="Tokyo, Japan")

    assert result.startswith("The current time in Tokyo, Japan is ")
    assert result.endswith(" JST.")


async def test_get_weather_returns_live_conditions(monkeypatch) -> None:
    async def fake_fetch_json(url, parameters):
        if url == agent_module.OPEN_METEO_GEOCODING_URL:
            return {
                "results": [
                    {
                        "name": "Pune",
                        "country": "India",
                        "latitude": 18.52,
                        "longitude": 73.86,
                    }
                ]
            }
        return {
            "current": {
                "temperature_2m": 25.0,
                "apparent_temperature": 26.0,
                "relative_humidity_2m": 70,
                "weather_code": 2,
                "wind_speed_10m": 10.0,
            }
        }

    monkeypatch.setattr(agent_module, "_fetch_json", fake_fetch_json)
    tool = _tool(Assistant(), "get_weather")

    result = await tool(city="Pune")

    assert result == (
        "Current weather in Pune, India: partly cloudy, 25 degrees Celsius, "
        "feels like 26 degrees, humidity 70 percent, and wind 10 kilometers "
        "per hour."
    )


async def test_search_database_finds_deepgram() -> None:
    tool = _tool(Assistant(), "search_database")

    result = await tool(query="Deepgram")

    assert "Deepgram Nova-3 performs speech-to-text" in result


# Agent behavior is covered by the simulations in scenarios.yaml, which run full
# conversations against the agent on LiveKit Cloud (see README.md). The eval
# below is kept as an example of the in-process testing framework
# (https://docs.livekit.io/agents/start/testing/) for turn-level checks that
# don't need a live session. Uncomment it and run `uv run pytest` to use it.
#
# import textwrap
#
# import pytest
# from livekit.agents import AgentSession, inference, llm
#
# from agent import Assistant
#
#
# def _judge_llm() -> llm.LLM:
#     return inference.LLM(model="openai/gpt-4.1-mini")
#
#
# @pytest.mark.asyncio
# async def test_offers_assistance() -> None:
#     """Evaluation of the agent's friendly nature."""
#     async with (
#         _judge_llm() as judge_llm,
#         AgentSession() as session,
#     ):
#         await session.start(Assistant())
#
#         # Run an agent turn following the user's greeting
#         result = await session.run(user_input="Hello")
#
#         # Evaluate the agent's response for friendliness
#         await (
#             result.expect.next_event()
#             .is_message(role="assistant")
#             .judge(
#                 judge_llm,
#                 intent=textwrap.dedent(
#                     """\
#                     Greets the user in a friendly manner.
#
#                     Optional context that may or may not be included:
#                     - Offer of assistance with any request the user may have
#                     - Other small talk or chit chat is acceptable, so long as it is friendly and not too intrusive
#                     """
#                 ),
#             )
#         )
#
#         # Ensures there are no function calls or other unexpected events
#         result.expect.no_more_events()
