import re
import matplotlib
matplotlib.use('Agg')  # Set the backend to Agg, which can save plots as image files
import matplotlib.pyplot as plt

# Read the text file and extract hash values
filename = '/data/trieHashimoto/th_experiment_data_3/logs/0225_setHead_100000_sender.txt'
hash_values = []

with open(filename, 'r') as file:
    for line in file:
        match = re.search(r'Hash: (0x[0-9a-fA-F]{64})', line)
        if match:
            # Normalize the hash values
            hash_value = int(match.group(1), 16) / (2**256 - 1)
            hash_values.append(hash_value)

# Plot a histogram of the hash values
plt.hist(hash_values, bins='auto', edgecolor='black')
plt.xlabel('Normalized Hash Value Range')
plt.ylabel('Frequency')
plt.title('Histogram of Normalized Hash Values')

# Save the histogram as a PNG image
output_image = 'hash_histogram.png'
plt.savefig(output_image, dpi=300, bbox_inches='tight')