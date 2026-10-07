package com.sneha.luckfoxwifisetup

import android.Manifest
import android.app.Activity
import android.bluetooth.BluetoothAdapter
import android.bluetooth.BluetoothDevice
import android.bluetooth.BluetoothManager
import android.bluetooth.BluetoothSocket
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.view.ViewGroup
import android.widget.*
import org.json.JSONObject
import java.io.BufferedReader
import java.io.InputStreamReader
import java.io.OutputStreamWriter
import java.util.UUID

class MainActivity : Activity() {
    private val deviceName = "Luckfox-WiFi-Setup"
    private val boardAddress = "74:24:CA:50:17:6A"
    private val sppUuid = UUID.fromString("00001101-0000-1000-8000-00805F9B34FB")
    private lateinit var adapter: BluetoothAdapter
    private lateinit var status: TextView
    private lateinit var deviceSpinner: Spinner
    private lateinit var ssidInput: EditText
    private lateinit var passwordInput: EditText
    private val devices = mutableListOf<BluetoothDevice>()
    private var receiverRegistered = false

    private val receiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            if (intent.action != BluetoothDevice.ACTION_FOUND) return
            @Suppress("DEPRECATION")
            val device = if (Build.VERSION.SDK_INT >= 33) {
                intent.getParcelableExtra(BluetoothDevice.EXTRA_DEVICE, BluetoothDevice::class.java)
            } else {
                intent.getParcelableExtra(BluetoothDevice.EXTRA_DEVICE)
            } ?: return
            // Some Luckfox images do not expose the configured local name to
            // Android discovery. Show discovered devices so the user can
            // select the board by its advertised name or Bluetooth address.
            if ((device.address.equals(boardAddress, ignoreCase = true) || device.name == deviceName || device.name == "BlueZ 5.65") &&
                devices.none { it.address == device.address }) {
                devices.add(device)
                refreshDevices()
                setStatus("Found ${device.name}. Select the Luckfox device, then pair it if Android asks.")
            }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val manager = getSystemService(BLUETOOTH_SERVICE) as BluetoothManager
        adapter = manager.adapter
        buildUi()
        requestBluetoothPermissions()
    }

    private fun buildUi() {
        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = android.view.Gravity.CENTER_HORIZONTAL
            setPadding(32, 80, 32, 32)
        }
        val title = TextView(this).apply {
            text = "Luckfox Wi-Fi Setup"
            textSize = 25f
            gravity = android.view.Gravity.CENTER
        }
        root.addView(title, match())
        root.addView(TextView(this).apply {
            text = "Connect your phone to Luckfox over Bluetooth, then send Wi-Fi settings."
            textSize = 16f
            gravity = android.view.Gravity.CENTER
            setPadding(0, 12, 0, 20)
        }, match())

        val find = Button(this).apply {
            text = "1. Find Luckfox"
            setOnClickListener { scanForLuckfox() }
        }
        root.addView(find, match())

        deviceSpinner = Spinner(this)
        root.addView(deviceSpinner, match())

        ssidInput = EditText(this).apply {
            hint = "Wi-Fi name (SSID)"
            setSingleLine(true)
        }
        root.addView(ssidInput, match())

        passwordInput = EditText(this).apply {
            hint = "Wi-Fi password"
            inputType = android.text.InputType.TYPE_CLASS_TEXT or
                android.text.InputType.TYPE_TEXT_VARIATION_PASSWORD
            setSingleLine(true)
        }
        root.addView(passwordInput, match())

        val send = Button(this).apply {
            text = "2. Send Wi-Fi settings to Luckfox"
            setOnClickListener { sendCredentials() }
        }
        root.addView(send, match())

        status = TextView(this).apply {
            textSize = 16f
            setPadding(0, 24, 0, 0)
            text = "Status: Bluetooth is not connected"
        }
        root.addView(status, match())
        setContentView(root)
    }

    private fun match(): LinearLayout.LayoutParams =
        LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)

    private fun requestBluetoothPermissions() {
        val permissions = if (Build.VERSION.SDK_INT >= 31) arrayOf(
            Manifest.permission.BLUETOOTH_SCAN,
            Manifest.permission.BLUETOOTH_CONNECT
        ) else arrayOf(Manifest.permission.ACCESS_FINE_LOCATION)
        val missing = permissions.filter { checkSelfPermission(it) != PackageManager.PERMISSION_GRANTED }
        if (missing.isNotEmpty()) requestPermissions(missing.toTypedArray(), 20)
    }

    private fun hasBluetoothPermission(): Boolean = Build.VERSION.SDK_INT < 31 ||
        (checkSelfPermission(Manifest.permission.BLUETOOTH_SCAN) == PackageManager.PERMISSION_GRANTED &&
            checkSelfPermission(Manifest.permission.BLUETOOTH_CONNECT) == PackageManager.PERMISSION_GRANTED)

    private fun scanForLuckfox() {
        if (!hasBluetoothPermission()) {
            requestBluetoothPermissions()
            return
        }
        if (!adapter.isEnabled) {
            startActivityForResult(Intent(BluetoothAdapter.ACTION_REQUEST_ENABLE), 21)
            return
        }
        devices.clear()
        // Android may hide a classic Bluetooth device from discovery while it
        // is already cached under a generic name such as "BlueZ 5.65". The
        // Luckfox MAC is fixed, so include it directly and let connect() test
        // the device instead of depending on discovery callbacks.
        devices.add(adapter.getRemoteDevice(boardAddress))
        @Suppress("DEPRECATION")
        adapter.bondedDevices.filter {
            it.address.equals(boardAddress, ignoreCase = true) ||
                it.name == deviceName || it.name == "BlueZ 5.65"
        }.forEach { bonded ->
            if (devices.none { it.address.equals(bonded.address, ignoreCase = true) }) {
                devices.add(bonded)
            }
        }
        refreshDevices()
        if (!receiverRegistered) {
            val filter = IntentFilter(BluetoothDevice.ACTION_FOUND)
            if (Build.VERSION.SDK_INT >= 33) registerReceiver(receiver, filter, RECEIVER_EXPORTED)
            else @Suppress("DEPRECATION") registerReceiver(receiver, filter)
            receiverRegistered = true
        }
        @Suppress("DEPRECATION")
        adapter.startDiscovery()
        setStatus("Scanning. Make sure Luckfox is powered and discoverable.")
    }

    private fun refreshDevices() {
        val names = devices.map {
            val displayName = if (it.address.equals(boardAddress, ignoreCase = true)) {
                deviceName
            } else {
                it.name ?: "Bluetooth device"
            }
            "$displayName (${it.address})"
        }
        deviceSpinner.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, names.ifEmpty { listOf("No Luckfox found yet") })
    }

    private fun sendCredentials() {
        val ssid = ssidInput.text.toString().trim()
        val password = passwordInput.text.toString()
        if (ssid.isEmpty() || password.length < 8) {
            setStatus("Enter the Wi-Fi name and a password of at least 8 characters.")
            return
        }
        val selectedIndex = deviceSpinner.selectedItemPosition
        if (selectedIndex !in devices.indices) {
            setStatus("Tap Find Luckfox and select the Luckfox device first.")
            return
        }
        val device = devices[selectedIndex]
        @Suppress("DEPRECATION") adapter.cancelDiscovery()
        setStatus("Connecting to Luckfox. Please wait...")
        Thread {
            var socket: BluetoothSocket? = null
            try {
                if (Build.VERSION.SDK_INT >= 31 &&
                    checkSelfPermission(Manifest.permission.BLUETOOTH_CONNECT) != PackageManager.PERMISSION_GRANTED) {
                    runOnUiThread { setStatus("Bluetooth permission is required.") }
                    return@Thread
                }
                runOnUiThread { setStatus("Opening the Luckfox Wi-Fi setup channel...") }
                // Use the board's direct insecure RFCOMM channel 4.
                @Suppress("JavaReflectionMemberVisibility")
                socket = device.javaClass
                    .getMethod("createInsecureRfcommSocket", Int::class.javaPrimitiveType)
                    .invoke(device, 4) as BluetoothSocket
                socket.connect()
                val writer = OutputStreamWriter(socket.outputStream, Charsets.UTF_8)
                val payload = JSONObject().put("ssid", ssid).put("password", password).toString()
                writer.write(payload + "\n")
                writer.flush()
                val reader = BufferedReader(InputStreamReader(socket.inputStream, Charsets.UTF_8))
                val replies = StringBuilder()
                val deadline = System.currentTimeMillis() + 60_000
                while (System.currentTimeMillis() < deadline) {
                    if (!reader.ready()) { Thread.sleep(250); continue }
                    val line = reader.readLine() ?: break
                    replies.append(line).append('\n')
                    if (line.startsWith("SUCCESS:") || line.startsWith("FAILED:")) break
                }
                val result = replies.toString().trim()
                runOnUiThread { setStatus(if (result.isEmpty()) "No reply from Luckfox." else result) }
            } catch (error: Exception) {
                runOnUiThread { setStatus("Bluetooth error: ${error.message ?: "connection failed"}") }
            } finally {
                try { socket?.close() } catch (_: Exception) { }
            }
        }.start()
    }

    private fun setStatus(message: String) {
        status.text = "Status: $message"
    }

    override fun onDestroy() {
        if (receiverRegistered) unregisterReceiver(receiver)
        super.onDestroy()
    }
}
