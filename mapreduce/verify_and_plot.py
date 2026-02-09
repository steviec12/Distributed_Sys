import time
import collections
import string
import matplotlib.pyplot as plt
import requests
import concurrent.futures

# Configuration
BUCKET = "hw4-mapreduce-stevi-1770254956"
FILE_KEY = "hamlet.txt"
SPLITTER_URL = "http://35.87.42.44:8080/split"
MAPPER_URLS = [
    "http://54.245.211.249:8081/map",
    "http://35.165.82.39:8081/map",
    "http://35.87.230.112:8081/map"
]
REDUCER_URL = "http://44.243.110.151:8082/reduce"

def count_words_local(filename):
    print(f"--- Running Local Count on {filename} ---")
    start_time = time.time()
    
    with open(filename, 'r') as f:
        text = f.read()
    
    text = text.lower()
    clean_words = []
    for w in text.split():
        w = w.strip("!\"#$%&'()*+,-./:;<=>?@[\]^_`{|}~")
        if w:
            clean_words.append(w)
            
    counts = collections.Counter(clean_words)
    end_time = time.time()
    duration = end_time - start_time
    print(f"Local count for 'a': {counts['a']}")
    print(f"Local time: {duration:.4f}s\n")
    return duration

def run_distributed_mapreduce():
    print(f"--- Running Distributed MapReduce (AWS) ---")
    start_total = time.time()

    # 1. Split
    print("1. Requesting Split...")
    start_split = time.time()
    resp = requests.get(f"{SPLITTER_URL}?bucket={BUCKET}&key={FILE_KEY}")
    if resp.status_code != 200:
        print("Split failed:", resp.text)
        return 0
    chunks = resp.json()['chunks'] # ["chunk-1.txt", "chunk-2.txt", "chunk-3.txt"]
    print(f"   Split done in {time.time() - start_split:.4f}s. Chunks: {chunks}")

    # 2. Map 
    print("2. Requesting Maps (Parallel)...")
    start_map = time.time()
    out_keys = []
    
    def call_mapper(url, chunk_key):
        r = requests.get(f"{url}?bucket={BUCKET}&key={chunk_key}")
        return r.json()['result']

    with concurrent.futures.ThreadPoolExecutor() as executor:
        # Map mapper URLs to chunks. If fewer mappers than chunks, cycle them.
        futures = []
        for i, chunk in enumerate(chunks):
            mapper_url = MAPPER_URLS[i % len(MAPPER_URLS)]
            futures.append(executor.submit(call_mapper, mapper_url, chunk))
        
        for future in concurrent.futures.as_completed(futures):
            out_keys.append(future.result())

    print(f"   Map done in {time.time() - start_map:.4f}s. Outputs: {out_keys}")

    # 3. Reduce
    print("3. Requesting Reduce...")
    start_reduce = time.time()
    keys_param = ",".join(out_keys)
    resp = requests.get(f"{REDUCER_URL}?bucket={BUCKET}&keys={keys_param}")
    print(f"   Reduce done in {time.time() - start_reduce:.4f}s")
    
    end_total = time.time()
    duration = end_total - start_total
    print(f"Total Distributed Time: {duration:.4f}s\n")
    return duration

def generate_plot(local_time, distributed_time):
    methods = ['Local (Single Machine)', 'Distributed (AWS Fargate)']
    times = [local_time, distributed_time]
    
    plt.figure(figsize=(10, 6))
    bars = plt.bar(methods, times, color=['green', 'orange'])
    
    for bar in bars:
        height = bar.get_height()
        plt.text(bar.get_x() + bar.get_width()/2., height,
                 f'{height:.4f} s',
                 ha='center', va='bottom')

    plt.ylabel('Processing Time (seconds)')
    plt.title('Performance Experiment: Local vs Cloud MapReduce')
    plt.suptitle(f'File: Hamlet.txt (~160KB). Network overhead dominates small files.', fontsize=10)
    plt.savefig('performance_experiment_real.png')
    print("Plot saved as performance_experiment_real.png")

if __name__ == "__main__":
   
    local_time = count_words_local("shakespeare-hamlet.txt")
    aws_time = run_distributed_mapreduce()
    
    generate_plot(local_time, aws_time)
