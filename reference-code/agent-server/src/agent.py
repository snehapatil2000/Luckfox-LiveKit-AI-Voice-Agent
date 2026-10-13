import asyncio
import json
import logging
import os
import re
import textwrap
import uuid
from collections.abc import Awaitable
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

from geckolink import GeckoLinkClient, GeckoLinkError

try:
    from livekit.plugins import sarvam
except ImportError:  # Keep English usable before the optional dependency is synced.
    sarvam = None

load_dotenv(".env.local")

logger = logging.getLogger("voice-transcript")

CONTROL_TOPIC = "luckfox-control"
ACTIVATION_EVENT_TYPE = "arm_question"
INTERRUPTION_EVENT_TYPE = "interrupt_answer"
MIC_SLEEP_EVENT_TYPE = "mic_sleep"
ASSISTANT_SPEAKING_EVENT_TYPE = "assistant_speaking"
ASSISTANT_LISTENING_EVENT_TYPE = "assistant_listening"
GECKOLINK_REQUEST_EVENT_TYPE = "geckolink_request"
GECKOLINK_RESPONSE_EVENT_TYPE = "geckolink_response"
GECKOLINK_REQUEST_TIMEOUT = 12.0
QUESTION_START_TIMEOUT = 1.5


KANNADA_SCRIPT = re.compile(r"[\u0C80-\u0CFF]")
LATIN_SCRIPT = re.compile(r"[A-Za-z]")
ENGLISH_REQUEST = re.compile(
    r"\bin\s+english\b|\b(?:english|inglish)(?:nalli|alli)\b|"
    r"(?:ಇಂಗ್ಲಿಷ್|ಆಂಗ್ಲ)[\u200c\u200d]*(?:ನಲ್ಲಿ|ದಲ್ಲಿ)"
)
KANNADA_REQUEST = re.compile(
    r"\bin\s+kannada\b|\bkannada(?:dalli|nalli|alli)\b|ಕನ್ನಡ(?:ದಲ್ಲಿ|ದಲ್ಲೇ|ದಲ್ಲಿ ಮಾತ್ರ)"
)
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


def select_response_language(user_text: str) -> Literal["en", "kn"]:
    """Use an explicit request, otherwise answer in the dominant input script."""
    normalized = user_text.casefold()
    if ENGLISH_REQUEST.search(normalized):
        return "en"
    if KANNADA_REQUEST.search(normalized):
        return "kn"
    kannada_characters = len(KANNADA_SCRIPT.findall(user_text))
    latin_characters = len(LATIN_SCRIPT.findall(user_text))
    return "kn" if kannada_characters > latin_characters else "en"


class RoomGeckoLinkTransport:
    """Send restricted GeckoLink requests through the connected Luckfox board."""

    def __init__(self, room):
        self._room = room
        self._pending: dict[str, asyncio.Future[dict[str, Any]]] = {}

    async def request(
        self, method: str, path: str, body: dict[str, Any] | None
    ) -> dict[str, Any]:
        request_id = uuid.uuid4().hex
        future = asyncio.get_running_loop().create_future()
        self._pending[request_id] = future
        packet = {
            "type": GECKOLINK_REQUEST_EVENT_TYPE,
            "request_id": request_id,
            "method": method,
            "path": path,
            "body": body,
        }
        try:
            await self._room.local_participant.publish_data(
                json.dumps(packet), reliable=True, topic=CONTROL_TOPIC
            )
            response = await asyncio.wait_for(future, timeout=GECKOLINK_REQUEST_TIMEOUT)
        except TimeoutError as error:
            raise GeckoLinkError(
                "The GeckoLink gateway did not respond. Check the Luckfox and A20 connection."
            ) from error
        finally:
            self._pending.pop(request_id, None)

        status = int(response.get("status") or 0)
        response_body = response.get("body")
        if status < 200 or status >= 300:
            detail = response.get("error") or ""
            if isinstance(response_body, dict):
                detail = (
                    response_body.get("message") or response_body.get("error") or detail
                )
            raise GeckoLinkError(
                f"GeckoLink rejected the request{': ' + str(detail) if detail else '.'}"
            )
        if not isinstance(response_body, dict):
            raise GeckoLinkError("The GeckoLink gateway returned an invalid response.")
        return response_body

    def handle_response(self, payload: dict[str, Any]) -> bool:
        if payload.get("type") != GECKOLINK_RESPONSE_EVENT_TYPE:
            return False
        future = self._pending.get(str(payload.get("request_id") or ""))
        if future is not None and not future.done():
            future.set_result(payload)
        return True

class Assistant(Agent):
    def __init__(
        self,
        english_tts,
        kannada_tts=None,
        on_sleep: Callable[[str], None] | None = None,
        geckolink: GeckoLinkClient | None = None,
    ):
        self._english_tts = english_tts
        self._kannada_tts = kannada_tts
        self._on_sleep = on_sleep
        self._geckolink = geckolink
        self._activation_armed = False
        self._activation_speech_started = False
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
                For every smart-home point, switch, light, scene, mood, or status
                request, always call the matching GeckoLink automation tool. Never
                claim that a device changed unless the tool reports success. Never
                invent a point ID. If a name is ambiguous, ask the user to choose
                from the matches returned by the tool.
                After controlling one point, report only that tool's result. Do not
                infer or announce the state of sibling switches, fans, or other
                points from conversation history. If the user asks for their state,
                call get_automation_point_status for each requested point first.
                A spoken multi-gang name such as "switch 3 one" means the single
                point "Sw3 - 1". Keep the device and gang together in one tool call;
                never split them into "switch 3" and "switch 1" calls.
                For a fan, use increase_fan_speed or decrease_fan_speed for relative
                changes. A requested fan speed or level from 1 to 4 means
                set_fan_speed with that exact value; never convert it to brightness.
                If the user names a device such as Sw2 in a fan-speed request, pass
                Sw2 as point_name; the tool will select its fan-capable point. Do not
                reinterpret Sw2 as the gang points Sw21 or Sw22.
                Reply in the same language as the user's current question, independent
                of earlier turns. If the user explicitly requests English or Kannada,
                use the requested language instead.
                Never invent tool results. Use the selected tool result in a short
                spoken answer. Call only the tool or tools needed for the request.
                """
            )
        )

    def arm_from_wake_word(self) -> None:
        """Allow exactly one question after local Edison detection."""
        self._activation_armed = True
        self._activation_speech_started = False
        logger.info("WAKE WORD: Edison accepted; speak within 1500 ms")

    def mark_activation_speech_started(self) -> bool:
        """Record speech start for the current wake-word activation."""
        if not self._activation_armed or self._activation_speech_started:
            return False
        self._activation_speech_started = True
        return True

    def cancel_activation(self) -> None:
        """Discard an old question without cancelling a new local wake."""
        self._activation_armed = False
        self._activation_speech_started = False

    def expire_activation(self) -> bool:
        """Return to standby if no speech begins after Edison."""
        if not self._activation_armed or self._activation_speech_started:
            return False
        self._activation_armed = False
        if self._on_sleep is not None:
            self._on_sleep("no_speech")
        return True

    def consume_activation(self) -> bool:
        """Consume and return the current one-question wake-word activation."""
        if not self._activation_armed or not self._activation_speech_started:
            return False
        self._activation_armed = False
        self._activation_speech_started = False
        if self._on_sleep is not None:
            self._on_sleep("question_complete")
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

    async def _automation_call(self, operation: Awaitable[str]) -> str:
        try:
            result = await operation
        except GeckoLinkError as error:
            result = str(error)
        except Exception:
            logger.exception("GeckoLink automation tool failed")
            result = "The automation request failed unexpectedly. Please try again."
        logger.info("TOOL RESULT: geckolink %s", result)
        return result

    @function_tool
    async def find_automation_points(self, query: str = "", room: str = "") -> str:
        """Find GeckoLink switches, lights, or other controllable points by name.

        Use this before control when the user gives an unclear point name, or
        when the user asks which devices are available.

        Args:
            query: Optional point or device name to search for.
            room: Optional room name used to narrow the search.
        """
        logger.info("TOOL CALL: find_automation_points query=%s room=%s", query, room)
        if self._geckolink is None:
            return "GeckoLink automation is not connected."
        return await self._automation_call(self._geckolink.find_points(query, room))

    @function_tool
    async def control_automation_point(
        self,
        point_name: str,
        action: Literal[
            "on",
            "off",
            "toggle",
            "set_brightness",
            "increase_fan_speed",
            "decrease_fan_speed",
            "set_fan_speed",
        ],
        room: str = "",
        brightness: int | None = None,
        fan_speed: int | None = None,
    ) -> str:
        """Control one existing GeckoLink point such as a switch or light.

        Args:
            point_name: Human-friendly point name, such as ceiling light.
            action: Turn on/off, toggle, set brightness, increase/decrease fan
                speed, or set an exact fan speed.
            room: Optional room name to disambiguate duplicate point names.
            brightness: Required only for set_brightness; integer from 0 to 100.
            fan_speed: Required only for set_fan_speed; integer from 1 to 4.
        """
        logger.info(
            "TOOL CALL: control_automation_point point=%s action=%s room=%s brightness=%s fan_speed=%s",
            point_name,
            action,
            room,
            brightness,
            fan_speed,
        )
        if self._geckolink is None:
            return "GeckoLink automation is not connected."
        return await self._automation_call(
            self._geckolink.control_point(
                point_name, action, room, brightness, fan_speed
            )
        )

    @function_tool
    async def get_automation_point_status(self, point_name: str, room: str = "") -> str:
        """Get the live on, off, availability, and brightness status of a point.

        Args:
            point_name: Human-friendly point name.
            room: Optional room name to disambiguate duplicate point names.
        """
        logger.info(
            "TOOL CALL: get_automation_point_status point=%s room=%s",
            point_name,
            room,
        )
        if self._geckolink is None:
            return "GeckoLink automation is not connected."
        return await self._automation_call(
            self._geckolink.point_status(point_name, room)
        )

    @function_tool
    async def activate_automation_scene(self, scene_name: str) -> str:
        """Activate an existing GeckoLink scene by its human-friendly name.

        Args:
            scene_name: Existing scene name, such as Movie Night.
        """
        logger.info("TOOL CALL: activate_automation_scene scene=%s", scene_name)
        if self._geckolink is None:
            return "GeckoLink automation is not connected."
        return await self._automation_call(self._geckolink.activate_scene(scene_name))

    @function_tool
    async def activate_automation_mood(self, mood_name: str) -> str:
        """Activate an existing GeckoLink mood by its human-friendly name.

        Args:
            mood_name: Existing mood name, such as Relax.
        """
        logger.info("TOOL CALL: activate_automation_mood mood=%s", mood_name)
        if self._geckolink is None:
            return "GeckoLink automation is not connected."
        return await self._automation_call(self._geckolink.activate_mood(mood_name))

    async def on_user_turn_completed(self, turn_ctx, new_message):
        user_text = (new_message.raw_text_content or "").strip()
        if not self.consume_activation():
            logger.info("STANDBY: ignored speech; say Edison first")
            raise StopResponse()

        if not user_text:
            raise StopResponse()

        response_language = select_response_language(user_text)
        language_name = "Kannada" if response_language == "kn" else "English"
        new_message.content = [
            f"{user_text}\n\nAnswer this turn only in {language_name}. "
            "Do not copy the language used in earlier answers."
        ]
        if self._kannada_tts is not None:
            self.update_options(
                tts=(
                    self._kannada_tts
                    if response_language == "kn"
                    else self._english_tts
                )
            )
        logger.info(
            "WAKE WORD: accepted one question language=%s: %s",
            language_name,
            user_text,
        )


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
                "min_delay": QUESTION_START_TIMEOUT,
                "max_delay": QUESTION_START_TIMEOUT,
            },
            # Luckfox speaker audio is captured by its nearby microphone.
            # Do not use acoustic interruption: local "Edison" is the
            # only activation control, which prevents speaker echo/noise from
            # cutting an answer short.
            interruption={
                "mode": "adaptive",
                "enabled": False,
                # Keep the session stable while TTS plays; a later wake word
                # interrupts the old answer and arms the next question.
                "discard_audio_if_uninterruptible": False,
                "min_duration": 0.45,
                "min_words": 4,
                "resume_false_interruption": True,
            },
            preemptive_generation={"enabled": True},
        ),
    )
    loop = asyncio.get_running_loop()
    background_tasks: set[asyncio.Task[Any]] = set()
    geckolink_transport = RoomGeckoLinkTransport(ctx.room)
    geckolink_client = GeckoLinkClient(geckolink_transport.request)

    async def notify_mic_sleep(reason: str) -> None:
        try:
            await ctx.room.local_participant.publish_data(
                json.dumps({"type": MIC_SLEEP_EVENT_TYPE, "reason": reason}),
                reliable=True,
                topic=CONTROL_TOPIC,
            )
        except Exception:
            logger.exception("STANDBY: failed to send MIC1 sleep event")

    def enter_sleep(reason: str) -> None:
        session.input.set_audio_enabled(False)
        task = loop.create_task(notify_mic_sleep(reason))
        background_tasks.add(task)
        task.add_done_callback(background_tasks.discard)
        logger.info("STANDBY: audio input disabled; MIC1 sleep requested reason=%s", reason)

    session.input.set_audio_enabled(False)
    logger.info("STANDBY: cloud audio input disabled until local Edison detection")
    assistant = Assistant(
        english_tts,
        kannada_tts,
        on_sleep=enter_sleep,
        geckolink=geckolink_client,
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
        if assistant.expire_activation():
            logger.info("WAKE WORD: no speech within 1500 ms; standby")

    def record_speech_start() -> None:
        if assistant.mark_activation_speech_started():
            cancel_arm_timeout()
            logger.info("WAKE WORD: speech started; capturing until 1500 ms silence")

    @ctx.room.on("data_received")
    def on_data_received(packet):
        nonlocal arm_timeout
        if packet.topic != CONTROL_TOPIC:
            return
        try:
            payload = json.loads(packet.data.decode("utf-8"))
        except (AttributeError, UnicodeDecodeError, json.JSONDecodeError):
            logger.warning("CONTROL: ignored invalid data packet")
            return
        if geckolink_transport.handle_response(payload):
            return
        if payload.get("type") == INTERRUPTION_EVENT_TYPE:
            cancel_arm_timeout()
            assistant.cancel_activation()
            session.input.set_audio_enabled(False)
            session.interrupt(force=True)
            logger.info("WAKE WORD: force-stopped old answer; waiting for beep completion")
            return
        if payload.get("type") == ACTIVATION_EVENT_TYPE:
            speech = session.current_speech
            if speech is not None and not speech.interrupted:
                logger.info("WAKE WORD: interrupting current answer")
                session.interrupt(force=True)
            assistant.arm_from_wake_word()
            session.input.set_audio_enabled(True)
            cancel_arm_timeout()
            arm_timeout = loop.call_later(QUESTION_START_TIMEOUT, expire_silent_arm)

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
        event_type = {
            "speaking": ASSISTANT_SPEAKING_EVENT_TYPE,
            "listening": ASSISTANT_LISTENING_EVENT_TYPE,
        }.get(event.new_state)
        if event_type is None:
            return

        async def notify_board_state() -> None:
            try:
                await ctx.room.local_participant.publish_data(
                    json.dumps({"type": event_type}),
                    reliable=True,
                    topic=CONTROL_TOPIC,
                )
            except Exception:
                logger.exception("VOICE STATE: failed to notify Luckfox state=%s", event.new_state)

        task = loop.create_task(notify_board_state())
        background_tasks.add(task)
        task.add_done_callback(background_tasks.discard)

    await session.start(
        agent=assistant,
        room=ctx.room,
        room_options=room_io.RoomOptions(close_on_disconnect=False),
    )

    await ctx.connect()
    await notify_mic_sleep("startup")


if __name__ == "__main__":
    agents.cli.run_app(server)
