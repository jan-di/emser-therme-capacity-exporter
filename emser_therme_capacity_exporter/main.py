import time
from prometheus_client import start_http_server, Gauge, Summary
import truststore
import requests
from bs4 import BeautifulSoup
import argparse
import logging

class Exporter:
    def __init__(self, port: int, interval: int, url: str):
        self.capacity_gauge = Gauge('emser_therme_capacity_percentage', 'Current percentage of capacity of Emser Therme')
        self.scrape_duration_summary = Summary('emser_therme_scrape_duration_seconds', 'Duration of scraping to Emser Therme website')

        self.port = port
        self.interval = interval
        self.url = url

    def start(self):
        start_http_server(self.port)
        while True:
            self.scrape_capacity()
            time.sleep(self.interval)

    def scrape_capacity(self):
        with self.scrape_duration_summary.time():
            req = requests.get(self.url, timeout=10)
            req.raise_for_status()

            soup = BeautifulSoup(req.text, 'html.parser')
            capacity_element = soup.select_one('div.capacity--area > h3')
            capacity_percentage = int(capacity_element.text.strip().replace('%', ''))

        self.capacity_gauge.set(capacity_percentage)

def parse_args() -> argparse.Namespace:
    argparser = argparse.ArgumentParser(description="Emser Therme Capacity Exporter")

    argparser.add_argument(
        '--port',
        type=int,
        default=8838,
        help='Port to listen on (default: 8838)'
    )
    argparser.add_argument(
        '--interval',
        type=int,
        default=60,
        help='Scraping interval in seconds (default: 60)'
    )
    argparser.add_argument(
        '--url',
        type=str,
        default='https://www.emser-therme.de',
        help='URL of the Emser Therme website (default: https://www.emser-therme.de)'
    )

    return argparser.parse_args()


def main() -> None:
    truststore.inject_into_ssl()
    args = parse_args()

    logging.basicConfig(level=logging.INFO)
    logging.info(f'Starting Emser Therme Capacity Exporter on port {args.port}')

    exporter = Exporter(
        port=args.port,
        interval=args.interval,
        url=args.url,
    )
    exporter.start()

if __name__ == '__main__':
    main()