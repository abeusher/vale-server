import requests
import json
import sys

# The URL of the Go Echo service endpoint
#VALE_API_URL = "http://localhost:1323/vale/"
VALE_API_URL = "http://localhost:1323/vale-text/"

def analyze_markdown_file_json(file_path):
    """
    Reads a Markdown file and sends its content to the Vale web service for analysis.

    Args:
        file_path (str): The path to the Markdown file.
    """
    print(f"[*] Reading file: {file_path}")
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            content = f.read()
    except FileNotFoundError:
        print(f"[!] Error: The file '{file_path}' was not found.")
        sys.exit(1)
    except Exception as e:
        print(f"[!] An error occurred while reading the file: {e}")
        sys.exit(1)

    # The payload must be a JSON object with a 'data' key
    payload = {
        "data": content
    }

    # Headers with API key for authentication
    headers = {
        "Authorization": "Bearer secret-api-key-48316-48316"
    }

    print(f"[*] Sending content to Vale service at {VALE_API_URL}")
    try:
        # Send the POST request with the JSON payload and auth headers
        response = requests.post(VALE_API_URL, json=payload, headers=headers)
        
        # Check if the request was successful
        response.raise_for_status()

    except requests.exceptions.ConnectionError:
        print("\n[!] Connection Error: Could not connect to the Vale service.")
        print("    Please ensure the Go Echo application is running.")
        sys.exit(1)
    except requests.exceptions.RequestException as e:
        print(f"\n[!] An error occurred during the request: {e}")
        print(f"    Response Body: {response.text}")
        sys.exit(1)

    print("[*] Received response from Vale. Analysis results:")
    print("-" * 50)

    # Parse the JSON response and pretty-print it
    results = response.json()
    print(json.dumps(results, indent=2))

def analyze_markdown_file_text(file_path):
    """
    Reads a Markdown file and sends its content to the Vale web service for analysis.

    Args:
        file_path (str): The path to the Markdown file.
    """
    print(f"[*] Reading file: {file_path}")
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            content = f.read()
    except FileNotFoundError:
        print(f"[!] Error: The file '{file_path}' was not found.")
        sys.exit(1)
    except Exception as e:
        print(f"[!] An error occurred while reading the file: {e}")
        sys.exit(1)

    # The payload must be a JSON object with a 'data' key
    payload = {
        "data": content
    }

    # Headers with API key for authentication
    headers = {
        "Authorization": "Bearer secret-api-key-48316-48316"
    }

    print(f"[*] Sending content to Vale service at {VALE_API_URL}")
    try:
        # Send the POST request with the JSON payload and auth headers
        response = requests.post(VALE_API_URL, json=payload, headers=headers)
        
        # Check if the request was successful
        response.raise_for_status()

    except requests.exceptions.ConnectionError:
        print("\n[!] Connection Error: Could not connect to the Vale service.")
        print("    Please ensure the Go Echo application is running.")
        sys.exit(1)
    except requests.exceptions.RequestException as e:
        print(f"\n[!] An error occurred during the request: {e}")
        print(f"    Response Body: {response.text}")
        sys.exit(1)

    print("[*] Received response from Vale. Analysis results:")
    print("-" * 50)

    # Parse the JSON response and pretty-print it
    results = response.text
    print(f"|{results}|")
    #results = response.json()
    #print(json.dumps(results, indent=2))

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python post_to_vale.py <filename>")
        print("Example: python post_to_vale.py example_file.md")
        sys.exit(1)
    
    file_to_analyze = sys.argv[1]
    #analyze_markdown_file_json(file_to_analyze)
    analyze_markdown_file_text(file_to_analyze)