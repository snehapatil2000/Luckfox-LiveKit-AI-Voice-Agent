import asyncio
import json
import logging
import os
import re
import textwrap
import time
from datetime import datetime, timedelta, timezone
from typing import Any, Callable, Literal
from urllib.parse import urlencode
from urllib.request import urlopen

from dotenv import load_dotenv
from livekit import agents
from livekit.agents import (
    Agent,
    AgentServer,
    AgentSession,
    StopResponse,
    TurnHandlingOptions,
    function_tool,
    inference,
    room_io,
)
from livekit.plugins import deepgram, openai

try:
    from livekit.plugins import sarvam
except ImportError:  # Keep English usable before the optional dependency is synced.
    sarvam = None

load_dotenv(".env.local")

logger = logging.getLogger("voice-transcript")

BUTTON_TOPIC = "luckfox-control"
BUTTON_EVENT_TYPE = "arm_question"
MIC_SLEEP_EVENT_TYPE = "mic_sleep"
BUTTON_SPEECH_TIMEOUT = 3.0


KANNADA_SCRIPT = re.compile(r"[\u0C80-\u0CFF]")
SARVAM_ENABLED = bool(os.getenv("SARVAM_API_KEY"))
OPEN_METEO_GEOCODING_URL = "https://geocoding-api.open-meteo.com/v1/search"
OPEN_METEO_FORECAST_URL = "https://api.open-meteo.com/v1/forecast"
TIME_LOCATION_ALIASES = {
    "america": "Washington, United States",
    "india": "New Delhi, India",
    "uk": "London, United Kingdom",
    "united kingdom": "London, United Kingdom",
    "united states": "Washington, United States",
    "usa": "Washington, United States",
}
WEATHER_DESCRIPTIONS = {
    0: "clear sky",
    1: "mainly clear",
    2: "partly cloudy",
    3: "overcast",
    45: "foggy",
    48: "foggy with frost",
    51: "light drizzle",
    53: "moderate drizzle",
    55: "heavy drizzle",
    56: "light freezing drizzle",
    57: "heavy freezing drizzle",
    61: "light rain",
    63: "moderate rain",
    65: "heavy rain",
    66: "light freezing rain",
    67: "heavy freezing rain",
    71: "light snow",
    73: "moderate snow",
    75: "heavy snow",
    77: "snow grains",
    80: "light rain showers",
    81: "moderate rain showers",
    82: "heavy rain showers",
    85: "light snow showers",
    86: "heavy snow showers",
    95: "thunderstorm",
    96: "thunderstorm with light hail",
    99: "thunderstorm with heavy hail",
}
PROJECT_DATABASE = (
    {
        "name": "Luckfox Pico Pi A W",
        "details": (
            "The Luckfox Pico Pi A W is the ARM Linux hardware client. Its microphone "
            "captures speech, and its speaker or headphones play the voice response."
        ),
    },
    {
        "name": "LiveKit",
        "details": (
            "LiveKit Cloud is the real-time communication server. It routes live audio "
            "between the Luckfox board and the Python AI agent."
        ),
    },
    {
        "name": "Sarvam",
        "details": (
            "Sarvam Saaras v4 performs English and Kannada speech-to-text. "
            "Sarvam Bulbul v3 performs English and Kannada text-to-speech."
        ),
    },
    {
        "name": "DeepSeek",
        "details": (
            "DeepSeek V4 Flash is the large language model. It understands questions, "
            "chooses tools when needed, and generates answers."
        ),
    },
    {
        "name": "Cloud deployment",
        "details": (
            "The production Python agent runs in LiveKit Cloud. The Luckfox needs "
            "power, audio hardware, Wi-Fi, and internet, but normal use does not "
            "require the development PC or ADB."
        ),
    },
    {
        "name": "Push-button standby",
        "details": (
            "The assistant stays in standby and ignores speech until the physical "
            "button is pressed. One press arms exactly one question until speech is accepted."
        ),
    },
)


async def _fetch_json(url: str, parameters: dict[str, Any]) -> dict[str, Any]:
    request_url = f"{url}?{urlencode(parameters)}"

    def request() -> dict[str, Any]:
        with urlopen(request_url, timeout=8) as response:
            payload = json.load(response)
        if not isinstance(payload, dict):
            raise ValueError("Expected a JSON object")
        return payload

    return await asyncio.to_thread(request)


class Assistant(Agent):
    def __init__(
        self,
        english_tts,
        kannada_tts=None,
        on_sleep: Callable[[], None] | None = None,
    ):
        self._english_tts = english_tts
        self._kannada_tts = kannada_tts
        self._on_sleep = on_sleep
        self._button_armed = False
        self._button_speech_started = False
        super().__init__(
            instructions=textwrap.dedent(
                """
                You are a helpful voice assistant.
                Reply briefly and naturally for speech.
                For explanation requests, give a complete answer in four to
                six short spoken sentences. Do not stop after one sentence or
                ask whether the user wants you to continue.
                Do not use markdown, emojis, lists, or special formatting.
                For every arithmetic request, always call the calculator tool.
                For current time, date, or day requests, always call the
                get_current_time tool. If no location is given, use New Delhi,
                India. If only a country is given, use that country's capital city.
                For current weather requests, always call the get_weather tool.
                To find stored information about this voice-agent project, always
                call the search_database tool.
                Reply in the same language as the user. If the user speaks Kannada,
                answer in Kannada script. If the user speaks English, answer in English.
                Never invent tool results. Use the selected tool result in a short
                spoken answer. Call only the tool or tools needed for the request.
                """
            )
        )

    def arm_from_button(self) -> None:
        """Allow exactly one question after a hardware-button press."""
        self._button_armed = True
        self._button_speech_started = False
        logger.info("PUSH BUTTON: armed; speak within 3000 ms")

    def mark_button_speech_started(self) -> bool:
        """Record speech start for the currently armed question."""
        if not self._button_armed or self._button_speech_started:
            return False
        self._button_speech_started = True
        return True

    def expire_button_arm(self) -> bool:
        """Return to standby if no speech started after the button press."""
        if not self._button_armed or self._button_speech_started:
            return False
        self._button_armed = False
        if self._on_sleep is not None:
            self._on_sleep()
        return True

    def consume_button_arm(self) -> bool:
        """Consume and return the current one-question hardware arm."""
        if not self._button_armed or not self._button_speech_started:
            return False
        self._button_armed = False
        self._button_speech_started = False
        if self._on_sleep is not None:
            self._on_sleep()
        return True

    @function_tool
    async def calculator(
        self,
        operation: Literal["add", "subtract", "multiply", "divide"],
        first_number: float,
        second_number: float,
    ) -> str:
        """Calculate two numbers using the requested arithmetic operation.

        You must call this tool whenever the user asks to add, subtract,
        multiply, or divide numbers.

        Args:
            operation: The arithmetic operation to perform.
            first_number: The first number in the calculation.
            second_number: The second number in the calculation.
        """
        logger.info(
            "TOOL CALL: calculator operation=%s first_number=%s second_number=%s",
            operation,
            first_number,
            second_number,
        )

        if operation == "add":
            result = first_number + second_number
        elif operation == "subtract":
            result = first_number - second_number
        elif operation == "multiply":
            result = first_number * second_number
        elif second_number == 0:
            result_text = "Division by zero is not allowed."
            logger.info("TOOL RESULT: calculator %s", result_text)
            return result_text
        else:
            result = first_number / second_number

        formatted_result = f"{result:.10g}"
        result_text = f"The calculator result is {formatted_result}."
        logger.info("TOOL RESULT: calculator %s", result_text)
        return result_text

    @function_tool
    async def get_current_time(self, location: str) -> str:
        """Get the current local time, date, and day for any location.

        You must call this tool whenever the user asks for the current time,
        date, or day. If the user names only a country, pass its capital city.
        If no location was requested, pass New Delhi, India.

        Args:
            location: City and optional country, such as Tokyo, Japan. For a
                country, use its capital city.
        """
        requested_location = location.strip()
        lookup_location = TIME_LOCATION_ALIASES.get(
            requested_location.lower(), requested_location
        )
        logger.info("TOOL CALL: get_current_time location=%s", lookup_location)

        try:
            location_data = await _fetch_json(
                OPEN_METEO_GEOCODING_URL,
                {
                    "name": lookup_location,
                    "count": 1,
                    "language": "en",
                    "format": "json",
                },
            )
            locations = location_data.get("results") or []
            if not locations:
                result = f"I could not find a location named {location}."
                logger.info("TOOL RESULT: get_current_time %s", result)
                return result

            resolved_location = locations[0]
            timezone_data = await _fetch_json(
                OPEN_METEO_FORECAST_URL,
                {
                    "latitude": resolved_location["latitude"],
                    "longitude": resolved_location["longitude"],
                    "current": "temperature_2m",
                    "timezone": "auto",
                    "forecast_days": 1,
                },
            )
            utc_offset = int(timezone_data["utc_offset_seconds"])
            now = datetime.now(timezone(timedelta(seconds=utc_offset)))
            hour = now.strftime("%I").lstrip("0")
            local_time = (
                f"{hour}:{now.strftime('%M')} {now.strftime('%p')} on "
                f"{now.strftime('%A, %d %B %Y')}"
            )
            place = resolved_location["name"]
            country = resolved_location.get("country")
            if country:
                place = f"{place}, {country}"
            abbreviation = timezone_data.get("timezone_abbreviation")
            zone_text = f" {abbreviation}" if abbreviation else ""
            result = f"The current time in {place} is {local_time}{zone_text}."
        except (KeyError, OSError, TypeError, ValueError) as error:
            logger.warning("Time lookup failed for %s: %s", location, error)
            result = "I could not get the local time right now. Please try again."

        logger.info("TOOL RESULT: get_current_time %s", result)
        return result

    @function_tool
    async def get_weather(self, city: str) -> str:
        """Get live current weather for a city.

        You must call this tool whenever the user asks about current weather.

        Args:
            city: City and optional state or country, such as Pune, India.
        """
        logger.info("TOOL CALL: get_weather city=%s", city)

        try:
            location_data = await _fetch_json(
                OPEN_METEO_GEOCODING_URL,
                {"name": city, "count": 1, "language": "en", "format": "json"},
            )
            locations = location_data.get("results") or []
            if not locations:
                result = f"I could not find a location named {city}."
                logger.info("TOOL RESULT: get_weather %s", result)
                return result

            location = locations[0]
            weather_data = await _fetch_json(
                OPEN_METEO_FORECAST_URL,
                {
                    "latitude": location["latitude"],
                    "longitude": location["longitude"],
                    "current": (
                        "temperature_2m,apparent_temperature,"
                        "relative_humidity_2m,weather_code,wind_speed_10m"
                    ),
                    "timezone": "auto",
                    "forecast_days": 1,
                },
            )
            current = weather_data["current"]
            description = WEATHER_DESCRIPTIONS.get(
                int(current["weather_code"]), "unknown conditions"
            )
            place = location["name"]
            country = location.get("country")
            if country:
                place = f"{place}, {country}"

            result = (
                f"Current weather in {place}: {description}, "
                f"{current['temperature_2m']:g} degrees Celsius, feels like "
                f"{current['apparent_temperature']:g} degrees, humidity "
                f"{current['relative_humidity_2m']:g} percent, and wind "
                f"{current['wind_speed_10m']:g} kilometers per hour."
            )
        except (KeyError, OSError, TypeError, ValueError) as error:
            logger.warning("Weather lookup failed for %s: %s", city, error)
            result = "I could not get the weather right now. Please try again."

        logger.info("TOOL RESULT: get_weather %s", result)
        return result

    @function_tool
    async def search_database(self, query: str) -> str:
        """Search the local demonstration database for voice-agent project facts.

        Use this tool for stored information about Luckfox, LiveKit, Sarvam,
        DeepSeek, cloud deployment, or push-button standby.

        Args:
            query: The project information to search for.
        """
        logger.info("TOOL CALL: search_database query=%s", query)
        normalized_query = " ".join(re.findall(r"[a-z0-9]+", query.lower()))
        query_words = {
            word
            for word in normalized_query.split()
            if len(word) > 2 and word not in {"about", "database", "find", "search"}
        }

        matches: list[tuple[int, dict[str, str]]] = []
        for record in PROJECT_DATABASE:
            searchable = f"{record['name']} {record['details']}".lower()
            score = sum(word in searchable for word in query_words)
            if normalized_query and normalized_query in searchable:
                score += 3
            if score:
                matches.append((score, record))

        matches.sort(key=lambda match: match[0], reverse=True)
        if not matches:
            result = f"No project database result was found for {query}."
        else:
            details = " ".join(record["details"] for _, record in matches[:3])
            result = f"Database result: {details}"

        logger.info("TOOL RESULT: search_database %s", result)
        return result

    async def on_user_turn_completed(self, turn_ctx, new_message):
        user_text = (new_message.raw_text_content or "").strip()
        if not self.consume_button_arm():
            logger.info("STANDBY: ignored speech; press the button for one question")
            raise StopResponse()

        if not user_text:
            raise StopResponse()

        new_message.content = [user_text]
        if self._kannada_tts is not None:
            self.update_options(
                tts=(
                    self._kannada_tts
                    if KANNADA_SCRIPT.search(user_text)
                    else self._english_tts
                )
            )
        logger.info("PUSH BUTTON: accepted one question: %s", user_text)


server = AgentServer()


@server.rtc_session(agent_name=os.getenv("LIVEKIT_AGENT_NAME", "my-agent"))
async def my_agent(ctx: agents.JobContext):
    if SARVAM_ENABLED and sarvam is None:
        raise RuntimeError(
            "SARVAM_API_KEY is set, but the Sarvam plugin is not installed. Run: uv sync"
        )

    english_tts = (
        sarvam.TTS(
            target_language_code="en-IN",
            model="bulbul:v3",
            speaker="shubh",
            speech_sample_rate=22050,
            pace=1.0,
        )
        if SARVAM_ENABLED
        else deepgram.TTS(model="aura-2-andromeda-en")
    )

    kannada_tts = (
        sarvam.TTS(
            target_language_code="kn-IN",
            model="bulbul:v3",
            speaker="shubh",
            speech_sample_rate=22050,
            pace=1.0,
        )
        if SARVAM_ENABLED
        else None
    )
    session = AgentSession(
        stt=(
            sarvam.STT(
                language="unknown",
                model="saaras:v4",
                mode="transcribe",
                sample_rate=16000,
                high_vad_sensitivity=True,
            )
            if SARVAM_ENABLED
            else deepgram.STT(model="nova-3", language="en-US")
        ),
        llm=openai.LLM.with_deepseek(
            model="deepseek-v4-flash",
        ),
        tts=english_tts,
        turn_handling=TurnHandlingOptions(
            turn_detection=inference.TurnDetector(unlikely_threshold=0.4),
            # After speech starts, wait for three seconds of silence before
            # committing the question. This lets users pause naturally.
            endpointing={
                "mode": "fixed",
                "min_delay": BUTTON_SPEECH_TIMEOUT,
                "max_delay": BUTTON_SPEECH_TIMEOUT,
            },
            # Luckfox speaker audio is captured by its nearby microphone.
            # Do not use acoustic interruption: the button is the only
            # activation control, which prevents speaker echo/noise from
            # cutting an answer short.
            interruption={
                "mode": "adaptive",
                "enabled": False,
                # Keep the session stable while TTS plays; a later button
                # press arms the next question.
                "discard_audio_if_uninterruptible": False,
                "min_duration": 0.45,
                "min_words": 4,
                "resume_false_interruption": True,
            },
            preemptive_generation={"enabled": True},
        ),
    )
    loop = asyncio.get_running_loop()

    async def notify_mic_sleep() -> None:
        try:
            await ctx.room.local_participant.publish_data(
                json.dumps({"type": MIC_SLEEP_EVENT_TYPE}),
                reliable=True,
                topic=BUTTON_TOPIC,
            )
        except Exception:
            logger.exception("STANDBY: failed to send MIC1 sleep event")

    def enter_sleep() -> None:
        session.input.set_audio_enabled(False)
        loop.create_task(notify_mic_sleep())
        logger.info("STANDBY: audio input disabled; MIC1 sleep requested")

    session.input.set_audio_enabled(False)
    logger.info("STANDBY: audio input disabled until the push button is pressed")
    assistant = Assistant(
        english_tts,
        kannada_tts,
        on_sleep=enter_sleep,
    )
    arm_timeout: asyncio.TimerHandle | None = None

    def cancel_arm_timeout() -> None:
        nonlocal arm_timeout
        if arm_timeout is not None:
            arm_timeout.cancel()
            arm_timeout = None

    def expire_silent_arm() -> None:
        nonlocal arm_timeout
        arm_timeout = None
        if assistant.expire_button_arm():
            logger.info("PUSH BUTTON: no speech within 3000 ms; standby")

    def record_speech_start() -> None:
        if assistant.mark_button_speech_started():
            cancel_arm_timeout()
            logger.info("PUSH BUTTON: speech started; capturing until 3000 ms silence")

    @ctx.room.on("data_received")
    def on_data_received(packet):
        nonlocal arm_timeout
        if packet.topic != BUTTON_TOPIC:
            return
        try:
            payload = json.loads(packet.data.decode("utf-8"))
        except (AttributeError, UnicodeDecodeError, json.JSONDecodeError):
            logger.warning("PUSH BUTTON: ignored invalid control packet")
            return
        if payload.get("type") == BUTTON_EVENT_TYPE:
            speech = session.current_speech
            if speech is not None and not speech.interrupted:
                logger.info("PUSH BUTTON: interrupting current answer")
                session.interrupt(force=True)
            assistant.arm_from_button()
            session.input.set_audio_enabled(True)
            cancel_arm_timeout()
            arm_timeout = loop.call_later(
                BUTTON_SPEECH_TIMEOUT, expire_silent_arm
            )

    @session.on("user_state_changed")
    def on_user_state_changed(event):
        if event.new_state == "speaking":
            record_speech_start()

    @session.on("user_input_transcribed")
    def on_user_input_transcribed(event):
        if event.transcript.strip():
            record_speech_start()

    @session.on("conversation_item_added")
    def on_conversation_item_added(event):
        item = event.item
        if getattr(item, "role", None) != "assistant":
            return

        response = getattr(item, "text_content", None)
        if response:
            logger.info("VOICE ASSISTANT: %s", response.strip())

    @session.on("agent_state_changed")
    def on_agent_state_changed(event):
        logger.info("VOICE STATE: %s", event.new_state)

    await session.start(
        agent=assistant,
        room=ctx.room,
        room_options=room_io.RoomOptions(close_on_disconnect=False),
    )

    await ctx.connect()
    await notify_mic_sleep()


if __name__ == "__main__":
    agents.cli.run_app(server)
