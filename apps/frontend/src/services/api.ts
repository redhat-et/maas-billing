const API_BASE_URL = 'http://localhost:3001/api/v1';

class ApiService {
  private async fetch(endpoint: string, options: RequestInit = {}) {
    const url = `${API_BASE_URL}${endpoint}`;
    const response = await fetch(url, {
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
      ...options,
    });

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    const data = await response.json();
    return data.success ? data.data : data;
  }

  async getModels() {
    return this.fetch('/models');
  }

  async getLiveRequests() {
    return this.fetch('/metrics/live-requests');
  }

  async getDashboardStats() {
    return this.fetch('/metrics/dashboard');
  }

  async getMetrics(timeRange: string = '1h') {
    return this.fetch(`/metrics?timeRange=${timeRange}`);
  }

  async getPolicies() {
    return this.fetch('/policies');
  }

  async createPolicy(policy: any) {
    return this.fetch('/policies', {
      method: 'POST',
      body: JSON.stringify(policy),
    });
  }

  async updatePolicy(id: string, policy: any) {
    return this.fetch(`/policies/${id}`, {
      method: 'PUT',
      body: JSON.stringify(policy),
    });
  }

  async deletePolicy(id: string) {
    return this.fetch(`/policies/${id}`, {
      method: 'DELETE',
    });
  }

  async getRequestDetails(id: string) {
    return this.fetch(`/metrics/requests/${id}`);
  }

  async getPolicyStats() {
    return this.fetch('/metrics/policy-stats');
  }
}

const apiService = new ApiService();
export default apiService;