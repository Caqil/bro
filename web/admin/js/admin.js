// Admin Dashboard JavaScript
class AdminDashboard {
    constructor() {
        this.apiBaseUrl = '/api';
        this.currentUser = null;
        this.currentSection = 'dashboard';
        this.authToken = localStorage.getItem('adminToken');
        
        this.init();
    }

    init() {
        this.setupEventListeners();
        this.checkAuthentication();
        this.loadDashboardData();
    }

    setupEventListeners() {
        // Sidebar navigation
        document.querySelectorAll('.nav-link').forEach(link => {
            link.addEventListener('click', (e) => {
                e.preventDefault();
                const section = link.getAttribute('data-section');
                this.navigateToSection(section);
            });
        });

        // Sidebar toggle
        const sidebarToggle = document.getElementById('sidebarToggle');
        const mobileSidebarToggle = document.getElementById('mobileSidebarToggle');
        const sidebar = document.getElementById('sidebar');

        if (sidebarToggle) {
            sidebarToggle.addEventListener('click', () => {
                sidebar.classList.toggle('collapsed');
            });
        }

        if (mobileSidebarToggle) {
            mobileSidebarToggle.addEventListener('click', () => {
                sidebar.classList.toggle('open');
            });
        }

        // Search functionality
        const globalSearch = document.getElementById('globalSearch');
        if (globalSearch) {
            globalSearch.addEventListener('input', (e) => {
                this.handleGlobalSearch(e.target.value);
            });
        }

        // User menu
        const logoutBtn = document.getElementById('logoutBtn');
        if (logoutBtn) {
            logoutBtn.addEventListener('click', (e) => {
                e.preventDefault();
                this.logout();
            });
        }

        // Modal handlers
        document.addEventListener('click', (e) => {
            if (e.target.classList.contains('modal-close')) {
                this.closeModal(e.target.getAttribute('data-modal'));
            }
            if (e.target.classList.contains('modal')) {
                this.closeModal(e.target.id);
            }
        });

        // System health refresh
        const refreshHealth = document.getElementById('refreshHealth');
        if (refreshHealth) {
            refreshHealth.addEventListener('click', () => {
                this.loadSystemHealth();
            });
        }

        // Users section
        this.setupUsersSection();

        // Window resize handler
        window.addEventListener('resize', () => {
            this.handleResize();
        });
    }

    setupUsersSection() {
        const userSearch = document.getElementById('userSearch');
        const userStatusFilter = document.getElementById('userStatusFilter');
        const exportUsers = document.getElementById('exportUsers');
        const selectAllUsers = document.getElementById('selectAllUsers');

        if (userSearch) {
            userSearch.addEventListener('input', (e) => {
                this.filterUsers();
            });
        }

        if (userStatusFilter) {
            userStatusFilter.addEventListener('change', () => {
                this.filterUsers();
            });
        }

        if (exportUsers) {
            exportUsers.addEventListener('click', () => {
                this.exportUsersData();
            });
        }

        if (selectAllUsers) {
            selectAllUsers.addEventListener('change', (e) => {
                this.toggleAllUsers(e.target.checked);
            });
        }
    }

    async checkAuthentication() {
        if (!this.authToken) {
            this.redirectToLogin();
            return;
        }

        try {
            const response = await this.apiCall('/auth/validate', 'GET');
            if (response.success) {
                this.currentUser = response.data.user;
                this.updateUserDisplay();
            } else {
                this.redirectToLogin();
            }
        } catch (error) {
            console.error('Authentication check failed:', error);
            this.redirectToLogin();
        }
    }

    redirectToLogin() {
        // Implement login redirect logic
        window.location.href = '/login';
    }

    updateUserDisplay() {
        if (this.currentUser) {
            const userNameElement = document.querySelector('.user-name');
            if (userNameElement) {
                userNameElement.textContent = this.currentUser.name || 'Admin';
            }
        }
    }

    navigateToSection(section) {
        // Hide all sections
        document.querySelectorAll('.content-section').forEach(s => {
            s.classList.remove('active');
        });

        // Show target section
        const targetSection = document.getElementById(`${section}-section`);
        if (targetSection) {
            targetSection.classList.add('active');
        }

        // Update navigation
        document.querySelectorAll('.nav-link').forEach(link => {
            link.classList.remove('active');
        });
        
        const activeLink = document.querySelector(`[data-section="${section}"]`);
        if (activeLink) {
            activeLink.classList.add('active');
        }

        // Update page title
        const pageTitle = document.getElementById('pageTitle');
        if (pageTitle) {
            pageTitle.textContent = this.capitalizeFirst(section);
        }

        this.currentSection = section;

        // Load section-specific data
        this.loadSectionData(section);
    }

    async loadSectionData(section) {
        switch (section) {
            case 'dashboard':
                await this.loadDashboardData();
                break;
            case 'users':
                await this.loadUsersData();
                break;
            case 'analytics':
                await this.loadAnalyticsData();
                break;
            case 'system':
                await this.loadSystemData();
                break;
        }
    }

    async loadDashboardData() {
        this.showLoading();
        
        try {
            const [dashboardStats, recentUsers, systemHealth] = await Promise.all([
                this.apiCall('/admin/dashboard', 'GET'),
                this.apiCall('/admin/users?limit=5&sort=created_at', 'GET'),
                this.apiCall('/admin/system/health', 'GET')
            ]);

            this.updateDashboardStats(dashboardStats.data);
            this.updateRecentUsers(recentUsers.data?.users || []);
            this.updateSystemHealth(systemHealth.data || systemHealth);
        } catch (error) {
            console.error('Failed to load dashboard data:', error);
            this.showError('Failed to load dashboard data');
        } finally {
            this.hideLoading();
        }
    }

    updateDashboardStats(stats) {
        if (!stats) return;

        // Update stat cards
        this.updateElement('totalUsers', stats.users?.total || 0);
        this.updateElement('newUsersToday', `${stats.users?.new_today || 0} new today`);
        this.updateElement('totalMessages', stats.messages?.total || 0);
        this.updateElement('messagesToday', `${stats.messages?.today || 0} today`);
        this.updateElement('totalGroups', stats.groups?.total || 0);
        this.updateElement('activeGroups', `${stats.groups?.active || 0} active`);
        this.updateElement('activeCalls', stats.calls?.active || 0);
        this.updateElement('callsToday', `${stats.calls?.total_today || 0} today`);
    }

    updateRecentUsers(users) {
        const tableBody = document.querySelector('#recentUsersTable tbody');
        if (!tableBody) return;

        tableBody.innerHTML = users.map(user => `
            <tr>
                <td>
                    <div class="user-info">
                        <img src="${user.avatar || 'https://via.placeholder.com/32'}" 
                             alt="${user.name}" class="user-avatar-small">
                        <span>${user.name}</span>
                    </div>
                </td>
                <td>${user.phone_number}</td>
                <td>
                    <span class="status-badge ${user.is_active ? 'active' : 'inactive'}">
                        ${user.is_active ? 'Active' : 'Inactive'}
                    </span>
                </td>
                <td>${this.formatDate(user.created_at)}</td>
            </tr>
        `).join('');
    }

    updateSystemHealth(health) {
        const healthContainer = document.getElementById('systemHealth');
        if (!healthContainer) return;

        const services = health.services || {};
        
        healthContainer.innerHTML = `
            <div class="health-item">
                <span>Overall Status</span>
                <div class="health-status-indicator ${health.status === 'healthy' ? '' : 'unhealthy'}"></div>
            </div>
            ${Object.entries(services).map(([service, status]) => `
                <div class="health-item">
                    <span>${this.capitalizeFirst(service)}</span>
                    <div class="health-status-indicator ${status.status === 'healthy' ? '' : 'unhealthy'}"></div>
                </div>
            `).join('')}
        `;
    }

    async loadUsersData() {
        this.showLoading();
        
        try {
            const response = await this.apiCall('/admin/users?page=1&limit=50', 'GET');
            this.updateUsersTable(response.data?.users || []);
            this.updateUsersPagination(response.meta || {});
        } catch (error) {
            console.error('Failed to load users data:', error);
            this.showError('Failed to load users data');
        } finally {
            this.hideLoading();
        }
    }

    updateUsersTable(users) {
        const tableBody = document.querySelector('#usersTable tbody');
        if (!tableBody) return;

        tableBody.innerHTML = users.map(user => `
            <tr>
                <td>
                    <input type="checkbox" class="user-checkbox" value="${user.id}">
                </td>
                <td>
                    <div class="user-info">
                        <img src="${user.avatar || 'https://via.placeholder.com/40'}" 
                             alt="${user.name}" class="user-avatar-small">
                        <div>
                            <div class="user-name">${user.name}</div>
                            <div class="user-email">${user.email || 'No email'}</div>
                        </div>
                    </div>
                </td>
                <td>${user.phone_number}</td>
                <td>
                    <span class="role-badge role-${user.role}">${user.role}</span>
                </td>
                <td>
                    <span class="status-badge ${user.is_active ? 'active' : 'inactive'}">
                        ${user.is_active ? 'Active' : 'Inactive'}
                    </span>
                </td>
                <td>${this.formatRelativeTime(user.last_seen)}</td>
                <td>${this.formatDate(user.created_at)}</td>
                <td>
                    <div class="action-buttons">
                        <button class="btn-small btn-primary" onclick="admin.viewUser('${user.id}')">
                            <i class="fas fa-eye"></i>
                        </button>
                        <button class="btn-small btn-warning" onclick="admin.editUser('${user.id}')">
                            <i class="fas fa-edit"></i>
                        </button>
                        <button class="btn-small btn-danger" onclick="admin.toggleUserStatus('${user.id}', ${user.is_active})">
                            <i class="fas fa-${user.is_active ? 'ban' : 'check'}"></i>
                        </button>
                    </div>
                </td>
            </tr>
        `).join('');
    }

    async loadAnalyticsData() {
        this.showLoading();
        
        try {
            const response = await this.apiCall('/admin/analytics?period=7d', 'GET');
            this.renderCharts(response.data);
        } catch (error) {
            console.error('Failed to load analytics data:', error);
            this.showError('Failed to load analytics data');
        } finally {
            this.hideLoading();
        }
    }

    renderCharts(data) {
        // User Growth Chart
        const userGrowthCtx = document.getElementById('userGrowthChart');
        if (userGrowthCtx && data.user_growth) {
            new Chart(userGrowthCtx, {
                type: 'line',
                data: {
                    labels: data.user_growth.growth_data?.map(d => this.formatChartDate(d._id)) || [],
                    datasets: [{
                        label: 'New Users',
                        data: data.user_growth.growth_data?.map(d => d.count) || [],
                        borderColor: '#667eea',
                        backgroundColor: 'rgba(102, 126, 234, 0.1)',
                        tension: 0.4
                    }]
                },
                options: {
                    responsive: true,
                    plugins: {
                        title: {
                            display: true,
                            text: 'User Growth'
                        }
                    }
                }
            });
        }

        // Message Volume Chart
        const messageVolumeCtx = document.getElementById('messageVolumeChart');
        if (messageVolumeCtx && data.message_volume) {
            new Chart(messageVolumeCtx, {
                type: 'bar',
                data: {
                    labels: data.message_volume.volume_data?.map(d => this.formatChartDate(d._id)) || [],
                    datasets: [{
                        label: 'Messages',
                        data: data.message_volume.volume_data?.map(d => d.count) || [],
                        backgroundColor: 'rgba(118, 75, 162, 0.8)'
                    }]
                },
                options: {
                    responsive: true,
                    plugins: {
                        title: {
                            display: true,
                            text: 'Message Volume'
                        }
                    }
                }
            });
        }
    }

    async loadSystemData() {
        this.showLoading();
        
        try {
            const [systemStatus, systemStats] = await Promise.all([
                this.apiCall('/admin/system/health', 'GET'),
                this.apiCall('/admin/stats', 'GET')
            ]);

            this.updateSystemStatus(systemStatus.data || systemStatus);
            this.updateSystemStats(systemStats.data);
        } catch (error) {
            console.error('Failed to load system data:', error);
            this.showError('Failed to load system data');
        } finally {
            this.hideLoading();
        }
    }

    updateSystemStatus(status) {
        const systemStatusContainer = document.getElementById('systemStatus');
        if (!systemStatusContainer) return;

        systemStatusContainer.innerHTML = `
            <div class="system-status">
                <h4>System Status: ${status.status}</h4>
                <p>Timestamp: ${this.formatDate(status.timestamp)}</p>
            </div>
        `;
    }

    // User Management Methods
    async viewUser(userId) {
        try {
            const response = await this.apiCall(`/admin/users/${userId}`, 'GET');
            this.openUserModal(response.data);
        } catch (error) {
            console.error('Failed to load user:', error);
            this.showError('Failed to load user details');
        }
    }

    openUserModal(userData) {
        const modal = document.getElementById('userModal');
        const modalBody = document.getElementById('userModalBody');
        
        modalBody.innerHTML = `
            <div class="user-details">
                <div class="user-avatar-large">
                    <img src="${userData.user?.avatar || 'https://via.placeholder.com/100'}" alt="${userData.user?.name}">
                </div>
                <div class="user-info-detailed">
                    <h3>${userData.user?.name}</h3>
                    <p><strong>Phone:</strong> ${userData.user?.phone_number}</p>
                    <p><strong>Email:</strong> ${userData.user?.email || 'Not provided'}</p>
                    <p><strong>Role:</strong> ${userData.user?.role}</p>
                    <p><strong>Status:</strong> ${userData.user?.is_active ? 'Active' : 'Inactive'}</p>
                    <p><strong>Last Seen:</strong> ${this.formatDate(userData.user?.last_seen)}</p>
                    <p><strong>Joined:</strong> ${this.formatDate(userData.user?.created_at)}</p>
                </div>
                ${userData.stats ? `
                    <div class="user-stats">
                        <h4>Statistics</h4>
                        <p><strong>Messages:</strong> ${userData.stats.message_count || 0}</p>
                        <p><strong>Groups:</strong> ${userData.stats.group_count || 0}</p>
                        <p><strong>Files:</strong> ${userData.stats.file_count || 0}</p>
                        <p><strong>Calls:</strong> ${userData.stats.call_count || 0}</p>
                    </div>
                ` : ''}
            </div>
        `;
        
        modal.classList.add('active');
    }

    async toggleUserStatus(userId, currentStatus) {
        const action = currentStatus ? 'ban' : 'unban';
        const confirmation = confirm(`Are you sure you want to ${action} this user?`);
        
        if (!confirmation) return;

        try {
            await this.apiCall(`/admin/users/${userId}/${action}`, 'POST', {
                reason: `${action} by admin`
            });
            
            this.showSuccess(`User ${action}ned successfully`);
            this.loadUsersData(); // Reload users table
        } catch (error) {
            console.error(`Failed to ${action} user:`, error);
            this.showError(`Failed to ${action} user`);
        }
    }

    async filterUsers() {
        const search = document.getElementById('userSearch')?.value || '';
        const status = document.getElementById('userStatusFilter')?.value || '';
        
        const params = new URLSearchParams();
        if (search) params.append('search', search);
        if (status) params.append('status', status);
        params.append('page', '1');
        params.append('limit', '50');

        try {
            const response = await this.apiCall(`/admin/users?${params.toString()}`, 'GET');
            this.updateUsersTable(response.data?.users || []);
            this.updateUsersPagination(response.meta || {});
        } catch (error) {
            console.error('Failed to filter users:', error);
        }
    }

    async exportUsersData() {
        try {
            this.showLoading();
            const response = await this.apiCall('/admin/users?format=csv', 'GET');
            
            // Create and download CSV
            const blob = new Blob([response], { type: 'text/csv' });
            const url = window.URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `users_export_${new Date().toISOString().split('T')[0]}.csv`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            window.URL.revokeObjectURL(url);
        } catch (error) {
            console.error('Failed to export users:', error);
            this.showError('Failed to export users data');
        } finally {
            this.hideLoading();
        }
    }

    // Utility Methods
    async apiCall(endpoint, method = 'GET', data = null) {
        const options = {
            method,
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${this.authToken}`
            }
        };

        if (data && (method === 'POST' || method === 'PUT')) {
            options.body = JSON.stringify(data);
        }

        const response = await fetch(this.apiBaseUrl + endpoint, options);
        
        if (!response.ok) {
            throw new Error(`API call failed: ${response.status}`);
        }

        return await response.json();
    }

    showLoading() {
        const overlay = document.getElementById('loadingOverlay');
        if (overlay) {
            overlay.classList.add('active');
        }
    }

    hideLoading() {
        const overlay = document.getElementById('loadingOverlay');
        if (overlay) {
            overlay.classList.remove('active');
        }
    }

    showError(message) {
        // Simple alert for now - could be replaced with a better notification system
        alert(`Error: ${message}`);
    }

    showSuccess(message) {
        // Simple alert for now - could be replaced with a better notification system
        alert(`Success: ${message}`);
    }

    openModal(modalId) {
        const modal = document.getElementById(modalId);
        if (modal) {
            modal.classList.add('active');
        }
    }

    closeModal(modalId) {
        const modal = document.getElementById(modalId);
        if (modal) {
            modal.classList.remove('active');
        }
    }

    updateElement(elementId, content) {
        const element = document.getElementById(elementId);
        if (element) {
            element.textContent = content;
        }
    }

    capitalizeFirst(str) {
        return str.charAt(0).toUpperCase() + str.slice(1);
    }

    formatDate(dateString) {
        if (!dateString) return 'Never';
        return new Date(dateString).toLocaleDateString('en-US', {
            year: 'numeric',
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit'
        });
    }

    formatRelativeTime(dateString) {
        if (!dateString) return 'Never';
        
        const now = new Date();
        const date = new Date(dateString);
        const diffMs = now - date;
        const diffMins = Math.floor(diffMs / 60000);
        const diffHours = Math.floor(diffMins / 60);
        const diffDays = Math.floor(diffHours / 24);

        if (diffMins < 5) return 'Just now';
        if (diffMins < 60) return `${diffMins}m ago`;
        if (diffHours < 24) return `${diffHours}h ago`;
        if (diffDays < 7) return `${diffDays}d ago`;
        
        return this.formatDate(dateString);
    }

    formatChartDate(dateObj) {
        if (!dateObj) return '';
        return `${dateObj.month}/${dateObj.day}`;
    }

    handleGlobalSearch(query) {
        // Implement global search functionality
        console.log('Searching for:', query);
    }

    handleResize() {
        // Handle responsive behavior
        const sidebar = document.getElementById('sidebar');
        if (window.innerWidth <= 768) {
            sidebar.classList.remove('open');
        }
    }

    toggleAllUsers(checked) {
        document.querySelectorAll('.user-checkbox').forEach(checkbox => {
            checkbox.checked = checked;
        });
    }

    updateUsersPagination(meta) {
        const paginationContainer = document.getElementById('usersPagination');
        if (!paginationContainer || !meta) return;

        const { page = 1, limit = 50, total = 0 } = meta;
        const totalPages = Math.ceil(total / limit);

        if (totalPages <= 1) {
            paginationContainer.innerHTML = '';
            return;
        }

        let paginationHTML = '';
        
        // Previous button
        if (page > 1) {
            paginationHTML += `<button onclick="admin.loadUsersPage(${page - 1})">Previous</button>`;
        }

        // Page numbers
        for (let i = Math.max(1, page - 2); i <= Math.min(totalPages, page + 2); i++) {
            paginationHTML += `<button class="${i === page ? 'active' : ''}" onclick="admin.loadUsersPage(${i})">${i}</button>`;
        }

        // Next button
        if (page < totalPages) {
            paginationHTML += `<button onclick="admin.loadUsersPage(${page + 1})">Next</button>`;
        }

        paginationContainer.innerHTML = paginationHTML;
    }

    async loadUsersPage(page) {
        const search = document.getElementById('userSearch')?.value || '';
        const status = document.getElementById('userStatusFilter')?.value || '';
        
        const params = new URLSearchParams();
        if (search) params.append('search', search);
        if (status) params.append('status', status);
        params.append('page', page.toString());
        params.append('limit', '50');

        try {
            this.showLoading();
            const response = await this.apiCall(`/admin/users?${params.toString()}`, 'GET');
            this.updateUsersTable(response.data?.users || []);
            this.updateUsersPagination(response.meta || {});
        } catch (error) {
            console.error('Failed to load users page:', error);
            this.showError('Failed to load users');
        } finally {
            this.hideLoading();
        }
    }

    async logout() {
        try {
            await this.apiCall('/auth/logout', 'POST');
        } catch (error) {
            console.error('Logout error:', error);
        } finally {
            localStorage.removeItem('adminToken');
            this.redirectToLogin();
        }
    }
}
// Admin Utilities JavaScript
// This file provides utility functions and enhancements for the admin dashboard

// Toast Notification System
class ToastManager {
    constructor() {
        this.container = this.createContainer();
        this.toasts = [];
    }

    createContainer() {
        let container = document.querySelector('.toast-container');
        if (!container) {
            container = document.createElement('div');
            container.className = 'toast-container';
            document.body.appendChild(container);
        }
        return container;
    }

    show(message, type = 'success', duration = 5000) {
        const toast = this.createToast(message, type);
        this.container.appendChild(toast);
        this.toasts.push(toast);

        // Trigger animation
        setTimeout(() => {
            toast.style.transform = 'translateX(0)';
        }, 10);

        // Auto remove
        setTimeout(() => {
            this.remove(toast);
        }, duration);

        return toast;
    }

    createToast(message, type) {
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        
        const icons = {
            success: 'fas fa-check',
            error: 'fas fa-times',
            warning: 'fas fa-exclamation-triangle',
            info: 'fas fa-info'
        };

        const titles = {
            success: 'Success',
            error: 'Error',
            warning: 'Warning',
            info: 'Information'
        };

        toast.innerHTML = `
            <div class="toast-icon">
                <i class="${icons[type]}"></i>
            </div>
            <div class="toast-content">
                <div class="toast-title">${titles[type]}</div>
                <div class="toast-message">${message}</div>
            </div>
            <button class="toast-close" onclick="toastManager.remove(this.closest('.toast'))">
                <i class="fas fa-times"></i>
            </button>
        `;

        return toast;
    }

    remove(toast) {
        if (toast && toast.parentNode) {
            toast.style.transform = 'translateX(100%)';
            setTimeout(() => {
                if (toast.parentNode) {
                    toast.parentNode.removeChild(toast);
                }
                const index = this.toasts.indexOf(toast);
                if (index > -1) {
                    this.toasts.splice(index, 1);
                }
            }, 300);
        }
    }

    success(message, duration) {
        return this.show(message, 'success', duration);
    }

    error(message, duration) {
        return this.show(message, 'error', duration);
    }

    warning(message, duration) {
        return this.show(message, 'warning', duration);
    }

    info(message, duration) {
        return this.show(message, 'info', duration);
    }
}

// Confirmation Dialog System
class ConfirmationDialog {
    constructor() {
        this.modal = this.createModal();
    }

    createModal() {
        const modal = document.createElement('div');
        modal.className = 'confirm-modal';
        modal.innerHTML = `
            <div class="confirm-modal-content">
                <div class="confirm-modal-icon">
                    <i class="fas fa-exclamation-triangle"></i>
                </div>
                <h3 class="confirm-modal-title">Confirm Action</h3>
                <p class="confirm-modal-message">Are you sure you want to perform this action?</p>
                <div class="confirm-modal-actions">
                    <button class="btn-secondary" id="confirmCancel">Cancel</button>
                    <button class="btn-danger" id="confirmOk">Confirm</button>
                </div>
            </div>
        `;
        document.body.appendChild(modal);

        // Close on backdrop click
        modal.addEventListener('click', (e) => {
            if (e.target === modal) {
                this.hide();
            }
        });

        return modal;
    }

    show(options = {}) {
        return new Promise((resolve) => {
            const {
                title = 'Confirm Action',
                message = 'Are you sure you want to perform this action?',
                confirmText = 'Confirm',
                cancelText = 'Cancel',
                type = 'danger'
            } = options;

            // Update content
            this.modal.querySelector('.confirm-modal-title').textContent = title;
            this.modal.querySelector('.confirm-modal-message').textContent = message;
            
            const confirmBtn = this.modal.querySelector('#confirmOk');
            const cancelBtn = this.modal.querySelector('#confirmCancel');
            
            confirmBtn.textContent = confirmText;
            cancelBtn.textContent = cancelText;
            confirmBtn.className = `btn-${type}`;

            // Set up event listeners
            const handleConfirm = () => {
                this.hide();
                resolve(true);
                cleanup();
            };

            const handleCancel = () => {
                this.hide();
                resolve(false);
                cleanup();
            };

            const cleanup = () => {
                confirmBtn.removeEventListener('click', handleConfirm);
                cancelBtn.removeEventListener('click', handleCancel);
            };

            confirmBtn.addEventListener('click', handleConfirm);
            cancelBtn.addEventListener('click', handleCancel);

            // Show modal
            this.modal.classList.add('active');
        });
    }

    hide() {
        this.modal.classList.remove('active');
    }
}

// Data Export Utility
class DataExporter {
    static exportToCSV(data, filename) {
        if (!Array.isArray(data) || data.length === 0) {
            toastManager.warning('No data to export');
            return;
        }

        // Get headers from first object
        const headers = Object.keys(data[0]);
        
        // Create CSV content
        let csvContent = headers.join(',') + '\n';
        
        data.forEach(row => {
            const values = headers.map(header => {
                let value = row[header] || '';
                // Escape quotes and commas
                if (typeof value === 'string') {
                    value = value.replace(/"/g, '""');
                    if (value.includes(',') || value.includes('"') || value.includes('\n')) {
                        value = `"${value}"`;
                    }
                }
                return value;
            });
            csvContent += values.join(',') + '\n';
        });

        // Download file
        this.downloadFile(csvContent, `${filename}.csv`, 'text/csv');
    }

    static exportToJSON(data, filename) {
        const jsonContent = JSON.stringify(data, null, 2);
        this.downloadFile(jsonContent, `${filename}.json`, 'application/json');
    }

    static downloadFile(content, filename, contentType) {
        const blob = new Blob([content], { type: contentType });
        const url = window.URL.createObjectURL(blob);
        const link = document.createElement('a');
        
        link.href = url;
        link.download = filename;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        window.URL.revokeObjectURL(url);

        toastManager.success(`File ${filename} downloaded successfully`);
    }
}

// Real-time Updates System
class RealTimeUpdater {
    constructor() {
        this.connections = new Map();
        this.updateInterval = 30000; // 30 seconds
        this.isActive = true;
    }

    start() {
        this.isActive = true;
        this.scheduleNext();
    }

    stop() {
        this.isActive = false;
        this.connections.clear();
    }

    register(key, callback, interval = this.updateInterval) {
        this.connections.set(key, {
            callback,
            interval,
            lastUpdate: 0
        });
    }

    unregister(key) {
        this.connections.delete(key);
    }

    async update() {
        if (!this.isActive) return;

        const now = Date.now();
        const promises = [];

        for (const [key, config] of this.connections) {
            if (now - config.lastUpdate >= config.interval) {
                promises.push(
                    config.callback().catch(error => {
                        console.error(`Update failed for ${key}:`, error);
                    })
                );
                config.lastUpdate = now;
            }
        }

        await Promise.all(promises);
    }

    scheduleNext() {
        if (!this.isActive) return;

        setTimeout(async () => {
            await this.update();
            this.scheduleNext();
        }, 5000); // Check every 5 seconds
    }
}

// Form Validation Utility
class FormValidator {
    static rules = {
        required: (value) => value && value.trim() !== '',
        email: (value) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value),
        phone: (value) => /^\+?[\d\s\-\(\)]+$/.test(value),
        minLength: (length) => (value) => value && value.length >= length,
        maxLength: (length) => (value) => value && value.length <= length,
        number: (value) => !isNaN(value) && !isNaN(parseFloat(value)),
        url: (value) => {
            try {
                new URL(value);
                return true;
            } catch {
                return false;
            }
        }
    };

    static validate(formData, validationRules) {
        const errors = {};

        for (const [field, rules] of Object.entries(validationRules)) {
            const value = formData[field];
            
            for (const rule of rules) {
                let isValid = false;
                let errorMessage = '';

                if (typeof rule === 'string') {
                    // Simple rule
                    isValid = this.rules[rule](value);
                    errorMessage = `${field} is ${rule}`;
                } else if (typeof rule === 'object') {
                    // Rule with parameters
                    const { type, message, ...params } = rule;
                    const ruleFunction = this.rules[type];
                    
                    if (ruleFunction) {
                        isValid = params ? ruleFunction(...Object.values(params))(value) : ruleFunction(value);
                        errorMessage = message || `${field} validation failed`;
                    }
                } else if (typeof rule === 'function') {
                    // Custom rule
                    isValid = rule(value);
                    errorMessage = `${field} is invalid`;
                }

                if (!isValid) {
                    errors[field] = errorMessage;
                    break; // Stop at first error for this field
                }
            }
        }

        return {
            isValid: Object.keys(errors).length === 0,
            errors
        };
    }

    static showErrors(errors) {
        // Clear previous errors
        document.querySelectorAll('.field-error').forEach(el => el.remove());

        // Show new errors
        for (const [field, message] of Object.entries(errors)) {
            const input = document.querySelector(`[name="${field}"], #${field}`);
            if (input) {
                const errorElement = document.createElement('div');
                errorElement.className = 'field-error';
                errorElement.textContent = message;
                errorElement.style.color = '#ef4444';
                errorElement.style.fontSize = '0.875rem';
                errorElement.style.marginTop = '0.25rem';
                
                input.parentNode.appendChild(errorElement);
                input.style.borderColor = '#ef4444';
            }
        }
    }
}

// Performance Monitor
class PerformanceMonitor {
    constructor() {
        this.metrics = {
            apiCalls: [],
            loadTimes: {},
            errors: []
        };
    }

    trackApiCall(endpoint, duration, status) {
        this.metrics.apiCalls.push({
            endpoint,
            duration,
            status,
            timestamp: Date.now()
        });

        // Keep only last 100 calls
        if (this.metrics.apiCalls.length > 100) {
            this.metrics.apiCalls.shift();
        }
    }

    trackLoadTime(section, duration) {
        this.metrics.loadTimes[section] = duration;
    }

    trackError(error, context) {
        this.metrics.errors.push({
            error: error.message || error,
            context,
            timestamp: Date.now()
        });

        // Keep only last 50 errors
        if (this.metrics.errors.length > 50) {
            this.metrics.errors.shift();
        }
    }

    getMetrics() {
        const now = Date.now();
        const oneMinuteAgo = now - 60000;

        return {
            recentApiCalls: this.metrics.apiCalls.filter(call => call.timestamp > oneMinuteAgo),
            averageApiResponseTime: this.calculateAverageResponseTime(),
            loadTimes: this.metrics.loadTimes,
            recentErrors: this.metrics.errors.filter(error => error.timestamp > oneMinuteAgo)
        };
    }

    calculateAverageResponseTime() {
        if (this.metrics.apiCalls.length === 0) return 0;
        
        const totalTime = this.metrics.apiCalls.reduce((sum, call) => sum + call.duration, 0);
        return Math.round(totalTime / this.metrics.apiCalls.length);
    }
}

// Initialize utilities
const toastManager = new ToastManager();
const confirmDialog = new ConfirmationDialog();
const realTimeUpdater = new RealTimeUpdater();
const performanceMonitor = new PerformanceMonitor();

// Enhanced API call function with performance tracking
async function apiCallWithTracking(endpoint, method = 'GET', data = null) {
    const startTime = performance.now();
    
    try {
        const options = {
            method,
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${localStorage.getItem('adminToken')}`
            }
        };

        if (data && (method === 'POST' || method === 'PUT')) {
            options.body = JSON.stringify(data);
        }

        const response = await fetch('/api' + endpoint, options);
        const duration = performance.now() - startTime;
        
        performanceMonitor.trackApiCall(endpoint, duration, response.status);
        
        if (!response.ok) {
            throw new Error(`API call failed: ${response.status}`);
        }

        return await response.json();
    } catch (error) {
        const duration = performance.now() - startTime;
        performanceMonitor.trackApiCall(endpoint, duration, 'error');
        performanceMonitor.trackError(error, `API call to ${endpoint}`);
        throw error;
    }
}

// Utility functions for common operations
const AdminUtils = {
    // Format file sizes
    formatFileSize: (bytes) => {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    },

    // Format numbers with commas
    formatNumber: (num) => {
        return num.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
    },

    // Debounce function
    debounce: (func, wait) => {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    },

    // Copy to clipboard
    copyToClipboard: async (text) => {
        try {
            await navigator.clipboard.writeText(text);
            toastManager.success('Copied to clipboard');
        } catch (error) {
            // Fallback for older browsers
            const textArea = document.createElement('textarea');
            textArea.value = text;
            document.body.appendChild(textArea);
            textArea.select();
            document.execCommand('copy');
            document.body.removeChild(textArea);
            toastManager.success('Copied to clipboard');
        }
    },

    // Generate random ID
    generateId: () => {
        return Date.now().toString(36) + Math.random().toString(36).substr(2);
    },

    // Validate admin permissions
    hasPermission: (requiredRole) => {
        const user = JSON.parse(localStorage.getItem('adminUser') || '{}');
        const roles = ['user', 'moderator', 'admin', 'super_admin'];
        const userRoleIndex = roles.indexOf(user.role);
        const requiredRoleIndex = roles.indexOf(requiredRole);
        return userRoleIndex >= requiredRoleIndex;
    }
};

// Export utilities for global access
window.toastManager = toastManager;
window.confirmDialog = confirmDialog;
window.realTimeUpdater = realTimeUpdater;
window.performanceMonitor = performanceMonitor;
window.DataExporter = DataExporter;
window.FormValidator = FormValidator;
window.AdminUtils = AdminUtils;
window.apiCallWithTracking = apiCallWithTracking;

// Initialize real-time updates when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    realTimeUpdater.start();
    
    // Register for dashboard updates
    if (window.admin) {
        realTimeUpdater.register('dashboard', () => {
            if (window.admin.currentSection === 'dashboard') {
                return window.admin.loadDashboardData();
            }
        }, 30000);
    }
});

// Clean up on page unload
window.addEventListener('beforeunload', () => {
    realTimeUpdater.stop();
});
// Initialize the admin dashboard when the page loads
let admin;
document.addEventListener('DOMContentLoaded', () => {
    admin = new AdminDashboard();
});

// Export for global access
window.admin = admin;